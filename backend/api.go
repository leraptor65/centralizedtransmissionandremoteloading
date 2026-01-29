package main

import (
	"encoding/json"
	"net/http"
)

func apiConfigHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		config := GetConfig()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(config)
		return
	}

	if r.Method == http.MethodPost {
		var newConfig Config
		if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		oldConfig := GetConfig()
		if err := UpdateConfig(newConfig); err != nil {
			http.Error(w, "Failed to save config", http.StatusInternalServerError)
			return
		}

		// Clear cookies if TargetURL changed
		if oldConfig.TargetURL != newConfig.TargetURL {
			for _, cookie := range r.Cookies() {
				// Expire cookie
				c := &http.Cookie{
					Name:     cookie.Name,
					Value:    "",
					Path:     "/",
					MaxAge:   -1,
					HttpOnly: false, // Try to clear both if possible, but we can't influence client-side only ones easily without name
				}
				http.SetCookie(w, c)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func apiReportHeightHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// We can log it or broadcast it via WebSocket if needed
	// For now just acknowledge
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"received"}`))
}

func apiVersionHandler(w http.ResponseWriter, r *http.Request) {
	config := GetConfig()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int64{"lastModified": config.LastModified})
}
