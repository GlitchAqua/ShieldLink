package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"shieldlink-server/internal/auth"
	"shieldlink-server/internal/client"
	"shieldlink-server/internal/config"
	"shieldlink-server/internal/log"
	"shieldlink-server/internal/merge"
	"shieldlink-server/internal/server"
)

func main() {
	configPath := flag.String("config", "", "local config file path")
	apiURL := flag.String("api", "", "API server URL to fetch config from")
	apiToken := flag.String("token", "", "API authentication token")
	mode := flag.String("mode", "", "override mode (server|merge)")
	hookMode := flag.Bool("hook", false, "hook mode: read YAML, start tunnels, write modified YAML")
	flag.Parse()

	// Hook mode: process a proxy config YAML for Android integration
	if *hookMode {
		if flag.NArg() < 2 {
			fmt.Fprintf(os.Stderr, "usage: shieldlink-server --hook <input.yaml> <output.yaml>\n")
			os.Exit(1)
		}
		runHook(flag.Arg(0), flag.Arg(1))
		return
	}

	var cfg *config.Config
	var err error

	switch {
	case *apiURL != "":
		// Load config from API server
		cfg, err = config.LoadFromAPI(*apiURL, *apiToken)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load config from API: %v\n", err)
			os.Exit(1)
		}
	case *configPath != "":
		// Load config from local file
		cfg, err = config.Load(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load config: %v\n", err)
			os.Exit(1)
		}
	default:
		// Try default path
		cfg, err = config.Load("config.json")
		if err != nil {
			fmt.Fprintf(os.Stderr, "usage: shieldlink-server --config <file> | --api <url> [--token <token>]\n")
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}

	if *mode != "" {
		cfg.Mode = *mode
	}

	// Init logger
	if cfg.Log.Enabled {
		log.Init(cfg.Log.Level, cfg.Log.File)
	} else {
		log.Init("silent", "")
	}

	log.L.Info("shieldlink starting",
		"mode", cfg.Mode,
		"version", "0.1.0",
		"protocol", cfg.Protocol,
	)

	// Start config poller if API is configured
	if cfg.API.URL != "" {
		interval := time.Duration(cfg.API.PollInterval) * time.Second
		config.StartConfigPoller(cfg.API.URL, cfg.API.Token, interval, func(newCfg *config.Config) {
			log.L.Info("config updated from API",
				"uuid", newCfg.UUID,
				"forward", newCfg.Forward,
			)
			// Hot-reload: currently only logs. Full reload requires restart.
			// Future: support graceful reload of forward address, uuid, etc.
		})
		log.L.Info("config poller started",
			"api", cfg.API.URL,
			"interval", interval,
		)
	}

	switch cfg.Mode {
	case "client":
		c := client.New(cfg)
		if err := c.Run(); err != nil {
			log.L.Error("client error", "err", err)
			os.Exit(1)
		}
	case "server":
		s := server.New(cfg)
		if cfg.AdminAddr != "" {
			go startAdminAPI(cfg.AdminAddr, cfg.AdminToken, s.Auth())
		}
		if err := s.Run(); err != nil {
			log.L.Error("server error", "err", err)
			os.Exit(1)
		}
	case "merge":
		m := merge.New(cfg)
		if cfg.AdminAddr != "" {
			go startMergeAdminAPI(cfg.AdminAddr, cfg.AdminToken, m)
		}
		if err := m.Run(); err != nil {
			log.L.Error("merge error", "err", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown mode: %s\n", cfg.Mode)
		os.Exit(1)
	}
}

// startAdminAPI starts a lightweight HTTP API for managing routes at runtime.
func startAdminAPI(addr, token string, ma *auth.MultiAuthenticator) {
	mux := http.NewServeMux()

	checkToken := func(w http.ResponseWriter, r *http.Request) bool {
		if token == "" {
			return true
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return false
		}
		return true
	}

	mux.HandleFunc("/api/routes", func(w http.ResponseWriter, r *http.Request) {
		if !checkToken(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")

		switch r.Method {
		case http.MethodGet:
			routes := ma.Routes()
			json.NewEncoder(w).Encode(routes)

		case http.MethodPut:
			body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			if err != nil {
				http.Error(w, `{"error":"read body"}`, http.StatusBadRequest)
				return
			}
			var req struct {
				Routes []auth.Route `json:"routes"`
			}
			if err := json.Unmarshal(body, &req); err != nil {
				http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
				return
			}
			ma.UpdateRoutes(req.Routes)
			log.L.Info("admin: routes updated", "count", len(req.Routes))
			json.NewEncoder(w).Encode(map[string]any{
				"message": fmt.Sprintf("updated %d routes", len(req.Routes)),
			})

		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		if !checkToken(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		routes := ma.Routes()
		json.NewEncoder(w).Encode(map[string]any{
			"status":      "ok",
			"route_count": len(routes),
		})
	})

	log.L.Info("admin API started", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.L.Error("admin API error", "err", err)
	}
}

// startMergeAdminAPI starts an admin API for the merge server to receive route updates.
func startMergeAdminAPI(addr, token string, m *merge.Merge) {
	mux := http.NewServeMux()

	checkToken := func(w http.ResponseWriter, r *http.Request) bool {
		if token == "" {
			return true
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return false
		}
		return true
	}

	mux.HandleFunc("/api/routes", func(w http.ResponseWriter, r *http.Request) {
		if !checkToken(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")

		switch r.Method {
		case http.MethodGet:
			json.NewEncoder(w).Encode([]map[string]string{
				{"uuid": "merge", "forward": m.GetForward()},
			})

		case http.MethodPut:
			body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			if err != nil {
				http.Error(w, `{"error":"read body"}`, http.StatusBadRequest)
				return
			}
			var req struct {
				Routes []struct {
					UUID    string `json:"uuid"`
					Forward string `json:"forward"`
				} `json:"routes"`
			}
			if err := json.Unmarshal(body, &req); err != nil {
				http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
				return
			}
			// Use the first route's forward as the merge forward target
			if len(req.Routes) > 0 {
				m.SetForward(req.Routes[0].Forward)
				log.L.Info("merge admin: forward updated", "forward", req.Routes[0].Forward)
			}
			json.NewEncoder(w).Encode(map[string]any{
				"message": fmt.Sprintf("updated forward to %d routes", len(req.Routes)),
			})

		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		if !checkToken(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":  "ok",
			"forward": m.GetForward(),
		})
	})

	log.L.Info("merge admin API started", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.L.Error("merge admin API error", "err", err)
	}
}
