package main

import (
	"log"
	"net/http"

	"aca/backend/internal/ai"
	"aca/backend/internal/analyzer"
	"aca/backend/internal/config"
	"aca/backend/internal/httpapi"
	"aca/backend/internal/scoring"
	"aca/backend/internal/store"
)

func main() {
	cfg := config.Load()
	st, err := store.New(cfg.DataDir)
	if err != nil {
		log.Fatal(err)
	}
	exec := ai.NewExecutor(cfg)
	scorer := scoring.New(exec)
	an := analyzer.New(cfg, scorer)
	server := httpapi.New(cfg, st, exec, scorer, an)
	log.Printf("ACA backend listening on :%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, server.Routes()))
}
