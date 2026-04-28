package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/SimonWaldherr/WatchSSH/internal/config"
	"github.com/SimonWaldherr/WatchSSH/internal/monitor"
	"github.com/SimonWaldherr/WatchSSH/internal/web"
)

const version = "2.0.0"

func main() {
	var (
		configFile  = flag.String("config", "config.yaml", "Path to the YAML configuration file")
		showVersion = flag.Bool("version", false, "Print version and exit")
		once        = flag.Bool("once", false, "Run a single collection cycle and exit")
		verbose     = flag.Bool("verbose", false, "Enable verbose logging")
		serverNames = flag.String("servers", "", "Comma-separated server names to monitor (default: all)")
		serverTags  = flag.String("tags", "", "Comma-separated server tags to monitor (default: all)")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("WatchSSH v%s\n", version)
		os.Exit(0)
	}

	if *verbose {
		log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	}

	_, statErr := os.Stat(*configFile)
	configFileExists := statErr == nil

	cfg, err := config.LoadOrDefault(*configFile)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	bootstrappedDiagnostics := ensureDiagnosticServer(cfg)
	cfg.Servers = filterServers(cfg.Servers, parseCSVSet(*serverNames), parseCSVSet(*serverTags))

	if bootstrappedDiagnostics && len(cfg.Servers) > 0 {
		if !configFileExists {
			log.Printf("No config file found at %q — starting with defaults.", *configFile)
			log.Printf("Open http://%s to configure WatchSSH via the web interface.", cfg.Web.Listen)
		}
		log.Printf("No servers configured — using the built-in localhost diagnostic profile (local metrics + Docker autodetect).")
	} else if !configFileExists {
		log.Printf("No config file found at %q — starting with defaults.", *configFile)
		log.Printf("Open http://%s to configure WatchSSH via the web interface.", cfg.Web.Listen)
	} else if len(cfg.Servers) == 0 {
		log.Printf("Warning: no servers matched the current configuration and filters.")
	}

	log.Printf("WatchSSH v%s starting — monitoring %d server(s)", version, len(cfg.Servers))
	if cfg.IsStrictHostKeyChecking() {
		log.Printf("Host key verification: strict (using %s)", knownHostsHint(cfg))
	} else {
		log.Printf("Host key verification: DISABLED (insecure mode)")
	}

	// Build the live state and wire up the notify callback for the web UI.
	state := web.NewState(cfg, *configFile)
	notifyFunc := monitor.NotifyFunc(func(metrics []monitor.ServerMetrics, firings []monitor.Firing) {
		state.Update(metrics, firings)
	})

	m := monitor.New(cfg, notifyFunc)

	if *once {
		// Single-poll mode: collect once, write output, exit.
		m.RunOnce()
		return
	}

	// Always start the web UI — it includes the configuration editor so the
	// user can set up WatchSSH even when no config file exists yet.
	webListen := cfg.Web.Listen
	if webListen == "" {
		webListen = ":8080"
	}
	webSrv := web.NewServer(state, webListen)
	go func() {
		if err := webSrv.Start(); err != nil {
			log.Printf("Web server stopped: %v", err)
		}
	}()
	log.Printf("Web dashboard available at http://%s", webListen)

	if len(cfg.Servers) > 0 {
		log.Printf("Polling every %ds", cfg.Interval)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go m.Start()

	sig := <-sigCh
	log.Printf("Received signal %v — shutting down", sig)
	m.Stop()
	log.Println("Goodbye.")
}

func knownHostsHint(cfg *config.Config) string {
	if cfg.KnownHostsPath != "" {
		return cfg.KnownHostsPath
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "~/.ssh/known_hosts"
	}
	return home + "/.ssh/known_hosts"
}

func parseCSVSet(v string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, item := range strings.Split(v, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out[item] = struct{}{}
	}
	return out
}

func ensureDiagnosticServer(cfg *config.Config) bool {
	if len(cfg.Servers) > 0 {
		return false
	}
	cfg.Servers = []config.Server{{
		Name:   "localhost",
		Local:  true,
		Tags:   []string{"local", "diagnostic"},
		Docker: config.DockerConfig{Enabled: true},
	}}
	return true
}

func filterServers(servers []config.Server, names map[string]struct{}, tags map[string]struct{}) []config.Server {
	if len(names) == 0 && len(tags) == 0 {
		return servers
	}
	filtered := make([]config.Server, 0, len(servers))
	for _, srv := range servers {
		if !matchesNameFilter(srv, names) {
			continue
		}
		if !matchesTagFilter(srv, tags) {
			continue
		}
		filtered = append(filtered, srv)
	}
	return filtered
}

func matchesNameFilter(srv config.Server, names map[string]struct{}) bool {
	if len(names) == 0 {
		return true
	}
	_, ok := names[srv.Name]
	return ok
}

func matchesTagFilter(srv config.Server, tags map[string]struct{}) bool {
	if len(tags) == 0 {
		return true
	}
	for _, tag := range srv.Tags {
		if _, ok := tags[tag]; ok {
			return true
		}
	}
	return false
}
