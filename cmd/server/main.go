package main

import (
	"log"
	"os"
	_ "time/tzdata"

	"github.com/haumanto/s3-usage-monitoring/internal/db"
	"github.com/haumanto/s3-usage-monitoring/internal/scheduler"
	"github.com/haumanto/s3-usage-monitoring/internal/web"
)

func main() {
	if err := db.Init(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	sch := scheduler.New()
	if err := sch.Start(); err != nil {
		log.Fatalf("Failed to start scheduler: %v", err)
	}
	defer sch.Stop()

	templateDir := os.Getenv("TEMPLATE_DIR")
	if templateDir == "" {
		templateDir = "./web/templates"
	}
	if err := web.InitTemplates(templateDir); err != nil {
		log.Fatalf("Failed to load templates: %v", err)
	}

	staticDir := os.Getenv("STATIC_DIR")
	if staticDir == "" {
		staticDir = "./web/static"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	if err := web.Run(":"+port, staticDir); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}