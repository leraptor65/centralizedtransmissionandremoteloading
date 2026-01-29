package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type HistoryItem struct {
	URL       string `yaml:"url" json:"url"`
	Timestamp int64  `yaml:"timestamp" json:"timestamp"`
}

type Config struct {
	TargetURL      string        `yaml:"targetUrl" json:"targetUrl"`
	ScaleFactor    float64       `yaml:"scaleFactor" json:"scaleFactor"`
	AutoScroll     bool          `yaml:"autoScroll" json:"autoScroll"`
	ScrollSpeed    int           `yaml:"scrollSpeed" json:"scrollSpeed"`
	ScrollSequence string        `yaml:"scrollSequence" json:"scrollSequence"`
	History        []HistoryItem `yaml:"history" json:"history"`
}

var (
	config       Config
	configMutex  sync.RWMutex
	dataDir      string
	settingsPath string
)

func initConfig() error {
	// Initialize default paths
	// In Docker, we map volume to /usr/src/app/data, but for local dev let's match existing
	ex, err := os.Getwd()
	if err != nil {
		return err
	}

	// Check if we are in backend dir or root
	if filepath.Base(ex) == "backend" {
		dataDir = filepath.Join(filepath.Dir(ex), "config_mount")
	} else {
		dataDir = filepath.Join(ex, "config_mount")
	}

	settingsPath = filepath.Join(dataDir, "settings.yml")

	// Ensure data directory exists
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			return fmt.Errorf("failed to create data directory: %w", err)
		}
	}

	// Default Config
	config = Config{
		TargetURL:   "https://github.com/leraptor65/centralizedtransmissionandremoteloading",
		ScaleFactor: 1.0,
		AutoScroll:  false,
		ScrollSpeed: 50,
		History:     []HistoryItem{},
	}

	return loadConfig()
}

func loadConfig() error {
	configMutex.Lock()
	defer configMutex.Unlock()

	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		// Create default settings.yml if it doesn't exist
		return saveConfigInternal()
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return fmt.Errorf("failed to read settings.yml: %w", err)
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse settings.yml: %w", err)
	}

	return nil
}

func GetConfig() Config {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return config
}

func UpdateConfig(newConfig Config) error {
	configMutex.Lock()
	defer configMutex.Unlock()

	// Update History logic similar to Node.js version
	if newConfig.TargetURL != "" && newConfig.TargetURL != config.TargetURL {
		// Add to history
		// Filter out existing
		newHistory := []HistoryItem{}
		newHistory = append(newHistory, HistoryItem{
			URL:       newConfig.TargetURL,
			Timestamp: time.Now().UnixMilli(),
		})

		for _, item := range config.History {
			if item.URL != newConfig.TargetURL {
				newHistory = append(newHistory, item)
			}
		}

		// Cap at 20
		if len(newHistory) > 20 {
			newHistory = newHistory[:20]
		}
		newConfig.History = newHistory
	} else {
		// Preserve history if not changing URL (e.g. just changing scale)
		// Unless the newConfig HAS history (e.g. restore/clear)
		// We trust the client sent the correct history history (including empty to clear)
	}

	config = newConfig
	return saveConfigInternal()
}

func saveConfigInternal() error {
	data, err := yaml.Marshal(&config)
	if err != nil {
		return err
	}
	return os.WriteFile(settingsPath, data, 0644)
}
