package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	configPath := flag.String("config", "/app/config.yaml", "path to detector config.yaml")
	flag.Parse()

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	auditor, err := NewAuditor(cfg.AuditLogPath)
	if err != nil {
		log.Fatalf("open audit log: %v", err)
	}
	defer auditor.Close()

	blocker, err := NewBlocker(cfg)
	if err != nil {
		log.Fatalf("init blocker: %v", err)
	}
	if cfg.StartupSelfCheck {
		if err := blocker.SelfCheck(); err != nil {
			log.Fatalf("iptables self-check failed: %v", err)
		}
	}

	notifier := NewNotifier(cfg)
	engine := NewEngine(cfg, blocker, notifier, auditor)
	dashboard := NewDashboardServer(cfg, engine)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go dashboard.Shutdown(ctx.Done())
	go func() {
		log.Printf("dashboard listening on %s", cfg.DashboardAddr)
		if err := dashboard.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("dashboard failed: %v", err)
		}
	}()

	log.Printf("detector running: log=%s audit=%s", cfg.LogPath, cfg.AuditLogPath)
	if err := engine.Run(ctx); err != nil && ctx.Err() == nil {
		log.Fatalf("engine stopped: %v", err)
	}
}
