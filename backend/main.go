package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
)

//go:embed all:frontend/dist
var frontendDist embed.FS

func main() {
	// 1. Initialize Config
	if err := initConfig(); err != nil {
		log.Fatalf("Failed to initialize config: %v", err)
	}
	log.Println("Configuration loaded.")

	// 2. Setup Router
	mux := http.NewServeMux()

	// API Routes
	mux.HandleFunc("/api/config", apiConfigHandler)
	mux.HandleFunc("/api/report-height", apiReportHeightHandler)
	mux.HandleFunc("/api/version", apiVersionHandler)

	// Proxy Handler (The "Catch All" for unhandled routes, unless we match specific prefixes)
	// We want to serve frontend for /config and /assets, and proxy logic for everything else
	// But /config is the dashboard.

	// Strategy:
	// If path starts with /api -> API
	// If path starts with /--proxy-host--/ -> Proxy
	// If path is /config -> Serve React App (HTML)
	// If path is / -> Proxy (Root) - Wait, in legacy, /config was the dashboard.
	// But usually / is the dashboard? No, in this app / is the proxied site (rewritten).
	// So we keep that. /config is the valid dashboard.

	// Static Files (Frontend)
	// We need to serve files from embedded fs "frontend/dist"
	// The React app will build to dist/index.html, dist/assets/...

	// Because we are embedding, we need to handle this carefully.
	// If we are developing locally without build, we might proxy to Vite dev server?
	// For production (docker), we assume embedded.

	// Let's create a handler for static files
	distFS, err := fs.Sub(frontendDist, "frontend/dist")
	if err != nil {
		// If fails (dev mode?), fallback or panic.
		// In dev mode, valid FS might not exist yet if not built.
		log.Println("Warning: frontend/dist not found (expected during dev before build)")
	}
	staticFileServer := http.FileServer(http.FS(distFS))

	proxy := newProxyHandler()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// 1. API - handled above? No, "/" matches everything in ServeMux if not specific.
		// So we dispatch manually here for strict ordering
		if len(path) >= 4 && path[:4] == "/api" {
			// standard mux would handle if we registered /api/
			// but we did /api/config.
			// Let's rely on standard mux logic?
			// Standard Mux matches most specific pattern.
			// So /api/config matches /api/config.
			// But /foo matches /.
			http.NotFound(w, r)
			return
		}

		// 2. Dashboard (/config)
		if path == "/config" {
			// Serve index.html
			// We need to serve the index.html from distFS
			if distFS != nil {
				data, err := fs.ReadFile(distFS, "index.html")
				if err == nil {
					w.Header().Set("Content-Type", "text/html")
					w.Write(data)
					return
				}
			}
			http.Error(w, "Dashboard not built", http.StatusServiceUnavailable)
			return
		}

		// 3. Static Assets (usually /assets/...)
		// Check if file exists in distFS
		if distFS != nil {
			// Clean path to remove leading /
			cleanPath := path[1:]
			if _, err := fs.Stat(distFS, cleanPath); err == nil {
				staticFileServer.ServeHTTP(w, r)
				return
			}
		}

		// 4. Proxy (Everything else)
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
