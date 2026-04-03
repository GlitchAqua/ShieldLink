# ShieldLink

轻量级加密中间件，透明嵌套在任何代理客户端和代理服务器之间，提供额外的加密层、多路径故障转移和带宽聚合能力。

## 特性

- **协议无关** — 兼容 SS、Trojan、VMess、VLESS、Hysteria2 等任何代理协议
- **透明加密** — TLS 1.3 / QUIC 加密隧道，不影响内层代理协议
- **多服务器故障转移** — 多台解密服务器互为主备，随机负载均衡，自动健康检查
- **带宽聚合** — 将流量拆分到多条路径，通过合流服务器重组，叠加带宽
- **MPTCP 支持** — 可选开启内核 MPTCP，多网卡聚合
- **mihomo 内核集成** — 直接嵌入 mihomo，无需独立进程，节点配置加一个字段即可启用
- **独立客户端模式** — 也可作为独立透明代理运行，兼容任何代理客户端
- **IP 透传** — 支持 PROXY Protocol v2，传递真实客户端 IP 到代理服务器
- **UUID 认证** — 两端通过 UUID 派生密钥，HMAC-SHA256 认证，防重放攻击
- **API 配置下发** — 服务端支持从 API 服务器拉取配置

## 架构

```
模式1 — 多路径故障转移:
[代理客户端] → [mihomo/ShieldLink] ═TLS/QUIC═> [解密服务器A] → [代理服务器]
                                   ═TLS/QUIC═> [解密服务器B]    (随机选择)

模式2 — 带宽聚合:
[代理客户端] → [mihomo/ShieldLink] ═拆流═> [解密服务器A] →┐
                                   ═拆流═> [解密服务器B] →├→ [合流服务器] → [代理服务器]
                                   ═拆流═> [解密服务器C] →┘
```

## 项目结构

```
shieldlink/
├── server/                     # shieldlink-server 独立二进制 (server/merge/client 三种模式)
│   ├── cmd/main.go
│   └── internal/
│       ├── auth/               # HMAC-SHA256 认证 + 重放检测
│       ├── config/             # 配置加载 (本地文件 / API 拉取)
│       ├── server/             # 解密服务端 (TCP/TLS + UDP/QUIC)
│       ├── merge/              # 合流服务端 (聚合帧重组)
│       ├── client/             # 独立客户端模式 (本地监听 + 隧道转发)
│       ├── relay/              # TCP/UDP 双向中继
│       ├── transport/          # TLS/QUIC 传输层
│       ├── protocol/           # 聚合帧协议
│       └── log/                # 日志系统
├── mihomo-patch/               # mihomo 内核补丁文件
│   ├── auth.go                 # 认证帧构建
│   ├── conn.go                 # 配置定义
│   ├── pool.go                 # 服务器池 + 健康检查 + TLS/QUIC 拨号
│   ├── listener.go             # 内嵌监听器 (故障转移 + 聚合)
│   ├── aggregate.go            # 拆流器 (AggregateWriter)
│   └── parser.go               # 节点解析器 (检测 shieldlink 字段，自动注入)
├── configs/                    # 示例配置
├── scripts/                    # 编译脚本
└── README.md
```

## 快速开始

### 1. 编译 shieldlink-server

```bash
cd server
go build -o shieldlink-server ./cmd/
```

### 2. 编译 mihomo-shieldlink 内核

```bash
# 克隆 mihomo 源码
git clone https://github.com/MetaCubeX/mihomo.git
cd mihomo
git checkout Alpha  # 或你需要的分支

# 应用 ShieldLink 补丁并编译
# 方式1: 使用脚本 (自动应用补丁 + 交叉编译7个平台)
../scripts/build-mihomo.sh .

# 方式2: 手动应用补丁
mkdir -p transport/shieldlink
cp ../mihomo-patch/auth.go transport/shieldlink/
cp ../mihomo-patch/conn.go transport/shieldlink/
cp ../mihomo-patch/pool.go transport/shieldlink/
cp ../mihomo-patch/listener.go transport/shieldlink/
cp ../mihomo-patch/aggregate.go transport/shieldlink/
cp ../mihomo-patch/parser.go adapter/parser.go

# 编译 (示例: Linux amd64)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags with_gvisor -trimpath -ldflags '-w -s' -o mihomo-shieldlink .

# 交叉编译其他平台
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -tags with_gvisor -trimpath -ldflags '-w -s' -o mihomo-shieldlink.exe .
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -tags with_gvisor -trimpath -ldflags '-w -s' -o mihomo-shieldlink-darwin .
CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -tags with_gvisor -trimpath -ldflags '-w -s' -o mihomo-shieldlink-android .
```

### 3. 部署解密服务器

在每台解密服务器上:

```bash
# TCP/TLS 模式
cat > config.json << 'EOF'
{
  "mode": "server",
  "listen": ":19443",
  "uuid": "your-shared-uuid",
  "protocol": "tcp",
  "tls": {"auto_cert": true},
  "forward": "代理服务器IP:端口",
  "ip_passthrough": false,
  "mptcp": true,
  "log": {"enabled": true, "level": "info"}
}
EOF
./shieldlink-server --config config.json

# UDP/QUIC 模式 (另一个端口)
cat > config-quic.json << 'EOF'
{
  "mode": "server",
  "listen": ":19445",
  "uuid": "your-shared-uuid",
  "protocol": "udp",
  "tls": {"auto_cert": true},
  "forward": "代理服务器IP:端口",
  "log": {"enabled": true, "level": "info"}
}
EOF
./shieldlink-server --config config-quic.json
```

### 4. 部署合流服务器 (仅聚合模式需要)

```bash
cat > merge.json << 'EOF'
{
  "mode": "merge",
  "listen": ":19000",
  "uuid": "your-shared-uuid",
  "forward": "代理服务器IP:端口",
  "log": {"enabled": true, "level": "info"},
  "reassembly": {"buffer_size": 4194304, "timeout": 5}
}
EOF
./shieldlink-server --config merge.json
```

聚合模式下，解密服务器的 `forward` 指向合流服务器地址（如 `merge-ip:19000`），合流服务器的 `forward` 指向最终代理服务器。

### 5. 客户端配置

#### 方式 A: mihomo 内核集成 (推荐)

替换 Clash Verge / Clash Meta 的 mihomo 内核为 `mihomo-shieldlink`，然后在节点配置中添加 `shieldlink` 字段:

```yaml
proxies:
  # TCP 故障转移
  - name: 'HK-01'
    type: ss
    server: proxy.example.com
    port: 30301
    cipher: chacha20-ietf-poly1305
    password: your-password
    udp: true
    shieldlink:
      uuid: your-shared-uuid
      servers:
        - address: decrypt-a.com:19443
          enabled: true
        - address: decrypt-b.com:19443
          enabled: true
      protocol: tcp
      transport: h2
      mptcp: true

  # UDP/QUIC 故障转移
  - name: 'HK-02-QUIC'
    type: ss
    server: proxy.example.com
    port: 30301
    cipher: chacha20-ietf-poly1305
    password: your-password
    udp: true
    shieldlink:
      uuid: your-shared-uuid
      servers:
        - address: decrypt-a.com:19445
          enabled: true
        - address: decrypt-b.com:19445
          enabled: true
      protocol: udp

  # TCP 带宽聚合
  - name: 'HK-03-AGG'
    type: ss
    server: proxy.example.com
    port: 30301
    cipher: chacha20-ietf-poly1305
    password: your-password
    udp: true
    shieldlink:
      uuid: your-shared-uuid
      servers:
        - address: decrypt-a.com:19444
          enabled: true
        - address: decrypt-b.com:19444
          enabled: true
      protocol: tcp
      transport: h2
      mptcp: true
      aggregate: true
      merge-address: merge-server.com:19000
```

原有节点的 `server`、`port`、`cipher`、`password` 等字段保持不变，只需追加 `shieldlink` 块。mihomo 内核会自动接管传输层。

#### 方式 B: 独立客户端

不改 mihomo 内核，单独运行 ShieldLink 客户端:

```bash
cat > client.json << 'EOF'
{
  "mode": "client",
  "listen": "127.0.0.1:30301",
  "uuid": "your-shared-uuid",
  "protocol": "tcp",
  "servers": [
    {"address": "decrypt-a.com:19443", "enabled": true},
    {"address": "decrypt-b.com:19443", "enabled": true}
  ],
  "log": {"enabled": true, "level": "info"}
}
EOF
./shieldlink-server --config client.json
```

然后将代理节点的 `server` 改为 `127.0.0.1`，`port` 改为 `30301`。

## 配置参考

### shieldlink 节点字段

| 字段 | 类型 | 说明 |
|---|---|---|
| `uuid` | string | UUID 标识，客户端和服务端必须一致 |
| `servers` | array | 解密服务器列表，互为主备随机选择 |
| `servers[].address` | string | 解密服务器地址 `host:port`，支持域名 |
| `servers[].enabled` | bool | 是否启用 |
| `protocol` | string | `tcp` (TLS 1.3) 或 `udp` (QUIC) |
| `transport` | string | TCP 模式的帧协议: `h2` 或 `ws`，仅 protocol=tcp 时有效 |
| `mptcp` | bool | 是否启用内核 MPTCP (Linux 5.6+) |
| `ip-passthrough` | bool | 是否启用 IP 透传 (PROXY Protocol v2) |
| `aggregate` | bool | 是否启用带宽聚合模式 |
| `merge-address` | string | 合流服务器地址，仅 aggregate=true 时需要 |

### server 配置

| 字段 | 说明 |
|---|---|
| `mode` | `server` / `merge` / `client` |
| `listen` | 监听地址 `:port` |
| `uuid` | UUID 标识 |
| `protocol` | `tcp` 或 `udp` |
| `tls.auto_cert` | 自动生成自签名证书 |
| `forward` | 转发目标地址 (代理服务器或合流服务器) |
| `ip_passthrough` | PROXY Protocol v2 IP 透传 |
| `mptcp` | 启用 MPTCP |
| `log.enabled` | 启用日志 |
| `log.level` | `debug` / `info` / `warn` / `error` / `silent` |
| `log.file` | 日志文件路径，空则输出到 stdout |
| `api.url` | API 服务器地址 (可选，定期拉取配置) |
| `api.token` | API 认证 token |
| `reassembly.buffer_size` | 合流重组缓冲区 (默认 4MB，仅 merge 模式) |
| `reassembly.timeout` | 合流重组超时秒数 (默认 5) |

## 端口规划示例

| 端口 | 协议 | 模式 | forward |
|---|---|---|---|
| 19443 | TCP/TLS | 故障转移 | 代理服务器 |
| 19444 | TCP/TLS | 聚合 | 合流服务器 |
| 19445 | UDP/QUIC | 故障转移 | 代理服务器 |
| 19446 | UDP/QUIC | 聚合 | 合流服务器 |
| 19000 | TCP | 合流 | 代理服务器 |

## 工作原理

### 认证协议

```
首包 (TLS/QUIC application data):
+──────────+──────────+────────+─────────+──────────+───────+────────────+──────────────+
| KEY_HINT | HMAC-256 | NONCE  | PAD_LEN | PADDING  | FLAGS | SESSION_ID | INITIAL_DATA |
| (4B)     | (32B)    | (8B)   | (2B)    | (var)    | (1B)  | (8B)       | (var)        |
+──────────+──────────+────────+─────────+──────────+───────+────────────+──────────────+
```

- KEY_HINT: SHA256(UUID) 前 4 字节，用于快速用户查找
- HMAC: HMAC-SHA256(key=SHA256(UUID), msg=NONCE)，认证
- NONCE: 时间戳(4B) + 随机(4B)，120 秒窗口 + 重放检测
- 认证通过后连接退化为纯双向管道，零额外开销

### 聚合帧

```
SESSION_ID(8B) + SEQ_NUM(4B) + CHUNK_LEN(2B) + DATA(var)
```

客户端将字节流拆分为块，round-robin 分发到多条路径，合流端按序号重组。

## License

MIT
