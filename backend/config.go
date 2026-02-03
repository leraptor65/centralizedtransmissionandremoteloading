package main

import (
	"log"
	"os"
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

func LoadConfig() Config {
	// Defaults
	targetURL := os.Getenv("TARGET_URL")
	if targetURL == "" {
		targetURL = "https://github.com/leraptor65"
	}

	scaleFactor, _ := strconv.ParseFloat(os.Getenv("SCALE_FACTOR"), 64)
	if scaleFactor <= 0 {
		scaleFactor = 1.0
	}
	if scaleFactor < 0.25 {
		scaleFactor = 0.25
	}
	if scaleFactor > 5.0 {
		scaleFactor = 5.0
	}

	autoScroll := os.Getenv("AUTO_SCROLL") == "true"
	scrollSpeed, _ := strconv.Atoi(os.Getenv("SCROLL_SPEED"))
	if scrollSpeed <= 0 {
		scrollSpeed = 10
	}

	autoReload := os.Getenv("AUTO_RELOAD") == "true"
	reloadInt, _ := strconv.Atoi(os.Getenv("RELOAD_INTERVAL"))
	if reloadInt <= 0 {
		reloadInt = 60
	}

	port := "1337"

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}

	// Ensure data directory exists for Chrome profile
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Printf("Warning: Could not create data dir: %v", err)
	}

	width, _ := strconv.Atoi(os.Getenv("WIDTH"))
	if width <= 0 {
		width = 1920
	}

	height, _ := strconv.Atoi(os.Getenv("HEIGHT"))
	if height <= 0 {
		height = 1080
	}

	return Config{
		TargetURL:   targetURL,
		ScaleFactor: scaleFactor,
		AutoScroll:  autoScroll,
		ScrollSpeed: scrollSpeed,
		DataDir:     dataDir,
		Port:        port,
		Width:       width,
		Height:      height,
		AutoReload:  autoReload,
		ReloadInt:   reloadInt,
	}
}
