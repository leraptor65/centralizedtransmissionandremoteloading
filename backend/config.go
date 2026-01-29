package main

import (
	"fmt"
	"net/http"
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

// Basic struct for JSON serialization of cookies
type Cookie struct {
	Name   string `json:"name" yaml:"name"`
	Value  string `json:"value" yaml:"value"`
	Domain string `json:"domain" yaml:"domain"`
	Path   string `json:"path" yaml:"path"`
}

type Config struct {
	TargetURL      string        `yaml:"targetUrl" json:"targetUrl"`
	ScaleFactor    float64       `yaml:"scaleFactor" json:"scaleFactor"`
	AutoScroll     bool          `yaml:"autoScroll" json:"autoScroll"`
	ScrollSpeed    int           `yaml:"scrollSpeed" json:"scrollSpeed"`
	ScrollSequence string        `yaml:"scrollSequence" json:"scrollSequence"`
	History        []HistoryItem `yaml:"history" json:"history"`
	LastModified   int64         `yaml:"lastModified" json:"lastModified"`
	CookieJar      []Cookie      `yaml:"cookieJar" json:"cookieJar"`
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
		TargetURL:    "https://github.com/leraptor65/centralizedtransmissionandremoteloading",
		ScaleFactor:  1.0,
		AutoScroll:   false,
		ScrollSpeed:  50,
		History:      []HistoryItem{},
		LastModified: time.Now().UnixMilli(),
		CookieJar:    []Cookie{},
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

	// Ensure LastModified is set if loading legacy config
	if config.LastModified == 0 {
		config.LastModified = time.Now().UnixMilli()
	}

	return nil
}

func GetConfig() Config {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return config
}

func UpdateCookies(cookies []*http.Cookie) {
	configMutex.Lock()
	defer configMutex.Unlock()

	updated := false
	existingMap := make(map[string]int)
	for i, c := range config.CookieJar {
		existingMap[c.Name] = i
	}

	for _, c := range cookies {
		// Simple logic: overwrite if name matches
		// Real logic should consider Domain/Path but for proxy usage usually strict.
		nc := Cookie{
			Name:   c.Name,
			Value:  c.Value,
			Domain: c.Domain,
			Path:   c.Path,
		}

		if idx, ok := existingMap[c.Name]; ok {
			if config.CookieJar[idx].Value != c.Value {
				config.CookieJar[idx] = nc
				updated = true
			}
		} else {
			config.CookieJar = append(config.CookieJar, nc)
			existingMap[c.Name] = len(config.CookieJar) - 1
			updated = true
		}
	}

	if updated {
		// We don't necessarily bump lastModified for cookies as it triggers reload loop
		// Only bump if we want to sync other settings.
		// For shared session, strictly backend sync is enough for next request.
		saveConfigInternal()
	}
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
		// Preserve history if not changing URL
		if len(newConfig.History) == 0 && len(config.History) > 0 {
			// Keep old history if new one is empty (unless intent was to clear, but we assume safety here)
			// Actually, typically the frontend sends the whole object.
			// Let's trust the frontend, but if it's missing, maybe copy?
			// But for safety, let's just stick to what was sent.
		}
	}

	// Always update LastModified
	newConfig.LastModified = time.Now().UnixMilli()

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
