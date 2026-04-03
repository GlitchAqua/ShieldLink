package main

import (
	"flag"
	"fmt"
	"os"
	"time"

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
	flag.Parse()

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
		if err := s.Run(); err != nil {
			log.L.Error("server error", "err", err)
			os.Exit(1)
		}
	case "merge":
		m := merge.New(cfg)
		if err := m.Run(); err != nil {
			log.L.Error("merge error", "err", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown mode: %s\n", cfg.Mode)
		os.Exit(1)
	}
}
