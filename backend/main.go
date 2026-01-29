package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
)

func main() {
	// 1. Initialize Config
	if err := initConfig(); err != nil {
		log.Fatalf("Failed to initialize config: %v", err)
	}
	log.Println("Configuration loaded from environment.")

	// 2. Setup Router
	mux := http.NewServeMux()

	// API Routes (keeping internal coordination ones)
	mux.HandleFunc("/api/report-height", apiReportHeightHandler)
	mux.HandleFunc("/api/version", apiVersionHandler)

	// Proxy Handler
	proxy := newProxyHandler()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// 1. Strict API handling
		if len(path) >= 4 && path[:4] == "/api" {
			// ServeMux will handle registered ones, others should be 404
			// However, / matches everything, so we need to be careful.
			// Handlers registered specifically (/api/...) take precedence.
			// But if we hit /api/foo (unregistered), it hits this / handler.
			http.NotFound(w, r)
			return
		}

		// 2. Proxy everything else
		proxy(w, r)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "1337"
	}

	log.Printf("Server listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}

func apiReportHeightHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Log height if needed, otherwise just success
	w.WriteHeader(http.StatusOK)
}

func apiVersionHandler(w http.ResponseWriter, r *http.Request) {
	config := GetConfig()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"lastModified": config.LastModified,
	})
}
