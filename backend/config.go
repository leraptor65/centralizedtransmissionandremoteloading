package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	TargetURL   string  `json:"targetUrl"`
	ScaleFactor float64 `json:"scaleFactor"`
	AutoScroll  bool    `json:"autoScroll"`
	ScrollSpeed int     `json:"scrollSpeed"` // Pixels per interval
	DataDir     string  `json:"-"`
	Port        string  `json:"-"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	AutoReload  bool    `json:"autoReload"`
	ReloadInt   int     `json:"reloadInt"` // Seconds
}

const configFileName = "config.json"

func LoadConfig() Config {
	// 1. Initialize with Defaults / Environment Variables
	cfg := Config{}

	cfg.TargetURL = os.Getenv("TARGET_URL")
	if cfg.TargetURL == "" {
		cfg.TargetURL = "https://github.com/leraptor65"
	}

	cfg.ScaleFactor, _ = strconv.ParseFloat(os.Getenv("SCALE_FACTOR"), 64)
	if cfg.ScaleFactor <= 0 {
		cfg.ScaleFactor = 1.0
	}
	if cfg.ScaleFactor < 0.25 {
		cfg.ScaleFactor = 0.25
	}
	if cfg.ScaleFactor > 5.0 {
		cfg.ScaleFactor = 5.0
	}

	cfg.AutoScroll = os.Getenv("AUTO_SCROLL") == "true"
	cfg.ScrollSpeed, _ = strconv.Atoi(os.Getenv("SCROLL_SPEED"))
	if cfg.ScrollSpeed <= 0 {
		cfg.ScrollSpeed = 10
	}

	cfg.AutoReload = os.Getenv("AUTO_RELOAD") == "true"
	cfg.ReloadInt, _ = strconv.Atoi(os.Getenv("RELOAD_INTERVAL"))
	if cfg.ReloadInt <= 0 {
		cfg.ReloadInt = 60
	}

	cfg.Port = "1337"

	cfg.DataDir = os.Getenv("DATA_DIR")
	if cfg.DataDir == "" {
		cfg.DataDir = "./data"
	}

	// Ensure data directory exists
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		log.Printf("Warning: Could not create data dir: %v", err)
	}

	cfg.Width, _ = strconv.Atoi(os.Getenv("WIDTH"))
	if cfg.Width <= 0 {
		cfg.Width = 1920
	}

	cfg.Height, _ = strconv.Atoi(os.Getenv("HEIGHT"))
	if cfg.Height <= 0 {
		cfg.Height = 1080
	}

	// 2. Load Overrides from Config File
	configPath := filepath.Join(cfg.DataDir, configFileName)
	if _, err := os.Stat(configPath); err == nil {
		file, err := os.ReadFile(configPath)
		if err == nil {
			var override Config
			if err := json.Unmarshal(file, &override); err == nil {
				// Apply overrides if present in JSON (zero value check might be tricky if 0 or false is valid,
				// but we assume the JSON contains the full valid state if it exists)
				// Actually, we should just overwrite with the struct from JSON, relying on it being complete-ish,
				// OR we carefully merge.
				// Since SaveConfig writes the FULL struct, we can just replace relevant fields.

				// We don't want to override DataDir or Port usually, but others yes.
				if override.TargetURL != "" {
					cfg.TargetURL = override.TargetURL
				}
				if override.ScaleFactor > 0 {
					cfg.ScaleFactor = override.ScaleFactor
				}
				cfg.AutoScroll = override.AutoScroll // bool, explicitly set
				if override.ScrollSpeed > 0 {
					cfg.ScrollSpeed = override.ScrollSpeed
				}
				cfg.AutoReload = override.AutoReload
				if override.ReloadInt > 0 {
					cfg.ReloadInt = override.ReloadInt
				}
				if override.Width > 0 {
					cfg.Width = override.Width
				}
				if override.Height > 0 {
					cfg.Height = override.Height
				}

				log.Printf("Loaded config overrides from %s", configPath)
			} else {
				log.Printf("Error parsing config file: %v", err)
			}
		}
	}

	return cfg
}

func SaveConfig(cfg Config) {
	// Ensure DataDir exists
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		log.Printf("Error creating data dir for config save: %v", err)
		return
	}

	configPath := filepath.Join(cfg.DataDir, configFileName)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		log.Printf("Error marshaling config: %v", err)
		return
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		log.Printf("Error saving config file: %v", err)
	} else {
		log.Printf("Configuration saved to %s", configPath)
	}
}

// ResetConfig removes the persistent config file
func ResetConfig() {
	// We need to know DataDir. Since we can't easily get it without loading,
	// checking env is safest or just checking ./data as fallback
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	configPath := filepath.Join(dataDir, configFileName)
	if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
		log.Printf("Error resetting config: %v", err)
	} else {
		log.Printf("Configuration reset (deleted %s)", configPath)
	}
}
