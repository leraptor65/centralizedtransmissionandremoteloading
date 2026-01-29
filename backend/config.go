package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

type Cookie struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
	Path   string `json:"path"`
}

type Config struct {
	TargetURL       string   `json:"targetUrl"`
	ScaleFactor     float64  `json:"scaleFactor"`
	AutoScroll      bool     `json:"autoScroll"`
	ScrollSpeed     int      `json:"scrollSpeed"`
	ScrollSequence  string   `json:"scrollSequence"`
	InterfaceLocked bool     `json:"interfaceLocked"`
	LastModified    int64    `json:"lastModified"`
	CookieJar       []Cookie `json:"cookieJar"`
}

var (
	config      Config
	configMutex sync.RWMutex
	startTime   int64
	dataDir     string
	cookiePath  string
)

func initConfig() error {
	startTime = time.Now().UnixMilli()

	// Setup Data Directory
	dataDir = os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	cookiePath = filepath.Join(dataDir, "cookies.json")

	// Ensure directory exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	targetURL := os.Getenv("TARGET_URL")
	if targetURL == "" {
		targetURL = "https://github.com/leraptor65/centralizedtransmissionandremoteloading"
	}

	scaleFactor, _ := strconv.ParseFloat(os.Getenv("SCALE_FACTOR"), 64)
	if scaleFactor <= 0 {
		scaleFactor = 1.0
	}

	autoScroll := os.Getenv("AUTO_SCROLL") == "true"
	scrollSpeed, _ := strconv.Atoi(os.Getenv("SCROLL_SPEED"))
	if scrollSpeed <= 0 {
		scrollSpeed = 50
	}

	config = Config{
		TargetURL:       targetURL,
		ScaleFactor:     scaleFactor,
		AutoScroll:      autoScroll,
		ScrollSpeed:     scrollSpeed,
		ScrollSequence:  os.Getenv("SCROLL_SEQUENCE"),
		InterfaceLocked: os.Getenv("INTERFACE_LOCKED") == "true",
		LastModified:    startTime,
		CookieJar:       []Cookie{},
	}

	// Load persistent cookies
	if err := loadCookies(); err != nil {
		fmt.Printf("Warning: failed to load cookies: %v\n", err)
	}

	return nil
}

func GetConfig() Config {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return config
}

func loadCookies() error {
	if _, err := os.Stat(cookiePath); os.IsNotExist(err) {
		return nil
	}
	data, err := os.ReadFile(cookiePath)
	if err != nil {
		return err
	}
	configMutex.Lock()
	defer configMutex.Unlock()
	return json.Unmarshal(data, &config.CookieJar)
}

func saveCookies() error {
	configMutex.RLock()
	data, err := json.MarshalIndent(config.CookieJar, "", "  ")
	configMutex.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(cookiePath, data, 0644)
}

func UpdateCookies(cookies []*http.Cookie) {
	configMutex.Lock()
	updated := false
	existingMap := make(map[string]int)
	for i, c := range config.CookieJar {
		existingMap[c.Name] = i
	}

	for _, c := range cookies {
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
	configMutex.Unlock()

	if updated {
		if err := saveCookies(); err != nil {
			fmt.Printf("Error saving cookies: %v\n", err)
		}
	}
}
