package web

import (
	"log"
	"net/http"
	"strings"
)

func NewServer(staticDir string) *http.ServeMux {
	mux := http.NewServeMux()

	// Pages
	mux.HandleFunc("/", DashboardHandler)
	mux.HandleFunc("/config", ConfigPageHandler)
	mux.HandleFunc("/settings", SettingsPageHandler)

	// API
	mux.HandleFunc("/api/accounts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			AccountsHandler(w, r)
		} else if r.Method == http.MethodPost {
			CreateAccountHandler(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/accounts/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/accounts/")
		if strings.Contains(path, "/") {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		switch r.Method {
		case http.MethodGet:
			GetAccountHandler(w, r)
		case http.MethodPost:
			UpdateAccountHandler(w, r)
		case http.MethodDelete:
			DeleteAccountHandler(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/settings", SettingsHandler)
	mux.HandleFunc("/api/trigger/", TriggerCheckHandler)
	mux.HandleFunc("/health", HealthHandler)

	// Static files
	fs := http.FileServer(http.Dir(staticDir))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	return mux
}

func Run(addr string, staticDir string) error {
	server := NewServer(staticDir)
	log.Printf("Server starting on %s", addr)
	return http.ListenAndServe(addr, server)
}
