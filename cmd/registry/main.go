// Command registry runs the Robot Agent Registry HTTP API server.
package main

import (
	"context"
	"log"

	"github.com/RussellTNY/robot-agent-registry/internal/auth"
	"github.com/RussellTNY/robot-agent-registry/internal/config"
	"github.com/RussellTNY/robot-agent-registry/internal/server"
	"github.com/RussellTNY/robot-agent-registry/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()
	st, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer st.Close()

	if err := st.Migrate(ctx); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	log.Println("migrations applied")

	am := auth.NewManager(cfg.JWTSigningKey)
	srv := server.New(cfg, st, am)

	log.Printf("Robot Agent Registry listening on :%s (base %s)", cfg.Port, cfg.BaseURL)
	if err := srv.Run(); err != nil {
		log.Fatalf("server: %v", err)
	}
}
