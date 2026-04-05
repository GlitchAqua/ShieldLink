package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"shieldlink-panel/internal/model"

	"golang.org/x/crypto/ssh"
)

const (
	remoteInstallDir = "/opt/shieldlink-server"
	remoteBinary     = remoteInstallDir + "/shieldlink-server"
	remoteConfig     = remoteInstallDir + "/config.json"
	processName      = "shieldlink-server"
	localBinDir      = "/root/shieldlink-server/bin"
)

var archBinaryMap = map[string]string{
	"x86_64":  "shieldlink-server-linux-amd64",
	"aarch64": "shieldlink-server-linux-arm64",
}

type CheckInstallResult struct {
	Installed bool   `json:"installed"`
	Action    string `json:"action"`
	Output    string `json:"output"`
	Error     string `json:"error,omitempty"`
}

func newSSHClient(cfg model.SSHInfo) (*ssh.Client, error) {
	host := cfg.GetSSHHost()
	if host == "" {
		return nil, fmt.Errorf("SSH host not configured")
	}
	port := cfg.GetSSHPort()
	if port <= 0 {
		port = 22
	}
	user := cfg.GetSSHUser()
	if user == "" {
		user = "root"
	}
	password := cfg.GetSSHPassword()
	if password == "" {
		return nil, fmt.Errorf("SSH password not configured")
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}

	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("SSH连接失败: %w", err)
	}
	return client, nil
}

func runSSH(client *ssh.Client, cmd string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("创建SSH会话失败: %w", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(cmd)
	return strings.TrimSpace(string(output)), err
}

func uploadFileViaSSH(client *ssh.Client, localPath, remotePath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("打开本地文件失败 %s: %w", localPath, err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return fmt.Errorf("获取文件信息失败: %w", err)
	}

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("创建SSH会话失败: %w", err)
	}
	defer session.Close()

	pipe, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("获取stdin管道失败: %w", err)
	}

	cmd := fmt.Sprintf("cat > %s", remotePath)
	if err := session.Start(cmd); err != nil {
		return fmt.Errorf("启动远程命令失败: %w", err)
	}

	written, err := io.Copy(pipe, f)
	if err != nil {
		return fmt.Errorf("传输文件失败: %w", err)
	}
	pipe.Close()

	if err := session.Wait(); err != nil {
		return fmt.Errorf("远程写入失败: %w", err)
	}

	if written != stat.Size() {
		return fmt.Errorf("传输不完整: 发送 %d / 总共 %d 字节", written, stat.Size())
	}

	return nil
}

// serverConfigJSON builds the shieldlink-server config.json content.
// For decrypt servers: mode=server, listen on the service port with TLS.
// For merge servers: mode=merge.
func serverConfigJSON(listenAddr, adminAddr, adminToken, mode, protocol string) ([]byte, error) {
	adminPort := "19480"
	if adminAddr != "" {
		if _, port, err := net.SplitHostPort(adminAddr); err == nil {
			adminPort = port
		}
	}

	// Extract listen port from address
	listenPort := "19443"
	if listenAddr != "" {
		if _, port, err := net.SplitHostPort(listenAddr); err == nil {
			listenPort = port
		}
	}

	// Always use "both" so server listens on TCP/TLS and UDP/QUIC simultaneously
	protocol = "both"

	cfg := map[string]interface{}{
		"mode":     mode,
		"listen":   "0.0.0.0:" + listenPort,
		"protocol": protocol,
		"tls": map[string]interface{}{
			"auto_cert": true,
		},
		"admin_addr":  ":" + adminPort,
		"admin_token": adminToken,
		"log": map[string]interface{}{
			"enabled": true,
			"level":   "info",
		},
	}

	if mode == "server" {
		cfg["routes"] = []map[string]string{
			{"uuid": "placeholder", "forward": "127.0.0.1:1"},
		}
	}
	if mode == "merge" {
		cfg["forward"] = "127.0.0.1:1"
		cfg["uuid"] = "placeholder"
	}

	return json.MarshalIndent(cfg, "", "  ")
}

// getServerMode returns the shieldlink-server mode.
// Decrypt servers: "server" (TLS relay with UUID auth)
// Merge servers: "merge" (plain TCP, receives aggregate frames and download channels)
func getServerMode(cfg model.SSHInfo) string {
	if _, ok := cfg.(*model.MergeServer); ok {
		return "merge"
	}
	return "server"
}

// getServerAddress gets the service listen address.
func getServerAddress(cfg model.SSHInfo) string {
	switch s := cfg.(type) {
	case *model.DecryptServer:
		return s.Address
	case *model.MergeServer:
		return s.Address
	}
	return ""
}

// getServerProtocol gets the protocol (tcp/udp) for decrypt servers.
func getServerProtocol(cfg model.SSHInfo) string {
	if s, ok := cfg.(*model.DecryptServer); ok {
		return s.Protocol
	}
	return "tcp"
}

// CheckAndInstall SSHes into a server, checks if shieldlink-server is running,
// and automatically uploads binary + configures + starts if not.
func CheckAndInstall(cfg model.SSHInfo) (*CheckInstallResult, error) {
	client, err := newSSHClient(cfg)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	// Step 1: Check if process is running
	output, err := runSSH(client, fmt.Sprintf("pgrep -f %s", processName))
	if err == nil && output != "" {
		pid := strings.Split(output, "\n")[0]
		return &CheckInstallResult{
			Installed: true,
			Action:    "already_installed",
			Output:    "进程运行中, PID: " + pid,
		}, nil
	}

	// Not running — auto install
	var logs []string
	logs = append(logs, "shieldlink-server 未运行，开始自动安装...")

	// Step 2: Detect remote architecture
	arch, err := runSSH(client, "uname -m")
	if err != nil {
		return nil, fmt.Errorf("检测远程架构失败: %w", err)
	}
	logs = append(logs, fmt.Sprintf("远程架构: %s", arch))

	// Step 3: Create install directory
	if _, err := runSSH(client, fmt.Sprintf("mkdir -p %s", remoteInstallDir)); err != nil {
		return nil, fmt.Errorf("创建目录失败: %w", err)
	}

	// Step 4: Check if binary already exists on remote
	_, binErr := runSSH(client, fmt.Sprintf("test -x %s", remoteBinary))
	if binErr != nil {
		binFile, ok := archBinaryMap[arch]
		if !ok {
			return &CheckInstallResult{
				Installed: false,
				Action:    "install_failed",
				Output:    strings.Join(logs, "\n"),
				Error:     fmt.Sprintf("不支持的架构: %s (仅支持 x86_64, aarch64)", arch),
			}, nil
		}

		localPath := localBinDir + "/" + binFile
		if _, err := os.Stat(localPath); os.IsNotExist(err) {
			return &CheckInstallResult{
				Installed: false,
				Action:    "install_failed",
				Output:    strings.Join(logs, "\n"),
				Error:     fmt.Sprintf("本地二进制文件不存在: %s，请先编译 shieldlink-server", localPath),
			}, nil
		}

		logs = append(logs, fmt.Sprintf("上传 %s ...", binFile))
		if err := uploadFileViaSSH(client, localPath, remoteBinary); err != nil {
			return &CheckInstallResult{
				Installed: false,
				Action:    "install_failed",
				Output:    strings.Join(logs, "\n"),
				Error:     fmt.Sprintf("上传失败: %v", err),
			}, nil
		}

		if _, err := runSSH(client, fmt.Sprintf("chmod +x %s", remoteBinary)); err != nil {
			return &CheckInstallResult{
				Installed: false,
				Action:    "install_failed",
				Output:    strings.Join(logs, "\n"),
				Error:     fmt.Sprintf("chmod 失败: %v", err),
			}, nil
		}
		logs = append(logs, "二进制文件就绪")
	} else {
		logs = append(logs, "二进制文件已存在，跳过上传")
	}

	// Step 5: Write config.json
	mode := getServerMode(cfg)
	address := getServerAddress(cfg)
	protocol := getServerProtocol(cfg)
	adminAddr := cfg.GetAdminAddr()
	adminToken := cfg.GetAdminToken()

	configData, err := serverConfigJSON(address, adminAddr, adminToken, mode, protocol)
	if err != nil {
		return &CheckInstallResult{
			Installed: false,
			Action:    "install_failed",
			Output:    strings.Join(logs, "\n"),
			Error:     fmt.Sprintf("生成配置失败: %v", err),
		}, nil
	}

	writeCmd := fmt.Sprintf("cat > %s << 'SLEOF'\n%s\nSLEOF", remoteConfig, string(configData))
	if _, err := runSSH(client, writeCmd); err != nil {
		return &CheckInstallResult{
			Installed: false,
			Action:    "install_failed",
			Output:    strings.Join(logs, "\n"),
			Error:     fmt.Sprintf("写入配置失败: %v", err),
		}, nil
	}

	adminPort := "19480"
	if adminAddr != "" {
		if _, port, err := net.SplitHostPort(adminAddr); err == nil {
			adminPort = port
		}
	}
	logs = append(logs, fmt.Sprintf("配置已写入 (mode=%s, admin=%s)", mode, adminPort))

	// Step 6: Kill any stale process and start fresh
	runSSH(client, fmt.Sprintf("pkill -f %s 2>/dev/null; sleep 1", processName))

	startCmd := fmt.Sprintf(
		"cd %s && nohup %s --config %s > %s/server.log 2>&1 & sleep 2 && pgrep -f %s",
		remoteInstallDir, remoteBinary, remoteConfig, remoteInstallDir, processName,
	)
	startOutput, startErr := runSSH(client, startCmd)
	if startErr != nil || startOutput == "" {
		// Try to get error from log
		logOut, _ := runSSH(client, fmt.Sprintf("tail -5 %s/server.log 2>/dev/null", remoteInstallDir))
		return &CheckInstallResult{
			Installed: false,
			Action:    "install_failed",
			Output:    strings.Join(append(logs, "启动输出: "+startOutput, "日志: "+logOut), "\n"),
			Error:     "启动 shieldlink-server 失败",
		}, nil
	}

	pid := strings.Split(startOutput, "\n")[0]
	logs = append(logs, fmt.Sprintf("启动成功, PID: %s", pid))

	return &CheckInstallResult{
		Installed: true,
		Action:    "reinstalled",
		Output:    strings.Join(logs, "\n"),
	}, nil
}
