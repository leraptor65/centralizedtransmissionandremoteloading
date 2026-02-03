package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

var (
	currentImg []byte
	imgMu      sync.RWMutex
	cfg        Config

	// State
	isLocked bool = false
	stateMu  sync.RWMutex
)

func main() {
	cfg = LoadConfig()
	log.Printf("Starting CTRL (Headless Mode) for: %s", cfg.TargetURL)

	// Ensure ctrl.sh exists in SCRIPT_DIR (or current dir)
	scriptDir := os.Getenv("SCRIPT_DIR")
	if scriptDir == "" {
		scriptDir = "."
	}
	ensureCtrlScript(scriptDir)

	// Cleanup bad lock file (fixes restart loop)
	cleanupSingletonLock(cfg.DataDir)

	// 1. Setup Chromedp
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-backgrounding-occluded-windows", true),
		chromedp.Flag("disable-renderer-backgrounding", true),
		chromedp.Flag("user-data-dir", cfg.DataDir),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	// 2. Initialize Browser
	// 2. Initialize Browser
	log.Printf("Initializing browser with Resolution: %dx%d, Scale: %f", cfg.Width, cfg.Height, cfg.ScaleFactor)
	logicalWidth := int64(float64(cfg.Width) / cfg.ScaleFactor)
	logicalHeight := int64(float64(cfg.Height) / cfg.ScaleFactor)

	if err := chromedp.Run(ctx,
		emulation.SetDeviceMetricsOverride(logicalWidth, logicalHeight, 1.0, false),
		chromedp.Navigate(cfg.TargetURL),
	); err != nil {
		log.Fatalf("Failed to start browser: %v", err)
	}

	// 3. Start Background Tasks
	go startScreenshotLoop(ctx)
	go startAutoScroll(ctx)
	go startAutoReload(ctx)

	// 4. HTTP ServerEndpoints
	http.HandleFunc("/", serveViewer)
	http.HandleFunc("/stream", serveStream)

	// --- Control API ---

	// Lock/Unlock
	http.HandleFunc("/lock", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			return
		}
		stateMu.Lock()
		isLocked = true
		stateMu.Unlock()

		// Force Reload to ensure clean state
		chromedp.Run(ctx, chromedp.Reload())

		log.Println("🔒 LOCKED")
		w.Write([]byte("Locked"))
	})
	http.HandleFunc("/unlock", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			return
		}
		stateMu.Lock()
		isLocked = false
		stateMu.Unlock()

		// Force Reload
		chromedp.Run(ctx, chromedp.Reload())

		log.Println("🔓 UNLOCKED")
		w.Write([]byte("Unlocked"))
	})

	// Status
	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		stateMu.RLock()
		defer stateMu.RUnlock()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"locked":      isLocked,
			"autoScroll":  cfg.AutoScroll,
			"scrollSpeed": cfg.ScrollSpeed,
			"scaleFactor": cfg.ScaleFactor,
			"targetUrl":   cfg.TargetURL,
			"autoReload":  cfg.AutoReload,
			"reloadInt":   cfg.ReloadInt,
		})
	})

	// Reload (Enhanced)
	http.HandleFunc("/reload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			return
		}

		// Check for args: ?state=on|off or ?interval=N
		q := r.URL.Query()
		state := q.Get("state")
		interval := q.Get("interval")

		stateMu.Lock()
		if state == "on" {
			cfg.AutoReload = true
		} else if state == "off" {
			cfg.AutoReload = false
		}

		if val, err := strconv.Atoi(interval); err == nil && val > 0 {
			cfg.ReloadInt = val
		}
		stateMu.Unlock()

		if err := chromedp.Run(ctx, chromedp.Reload()); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Write([]byte("Reloaded"))
	})

	// Config: AutoScroll
	http.HandleFunc("/config/autoscroll", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			return
		}
		// Query param enabled=true|false
		enabled := r.URL.Query().Get("enabled") == "true"
		stateMu.Lock()
		cfg.AutoScroll = enabled
		stateMu.Unlock()

		chromedp.Run(ctx, chromedp.Reload())

		log.Printf("AutoScroll set to %v", enabled)
		w.Write([]byte(fmt.Sprintf("AutoScroll: %v", enabled)))
	})

	// Config: Speed
	http.HandleFunc("/config/speed", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			return
		}
		val, _ := strconv.Atoi(r.URL.Query().Get("value"))
		if val > 0 {
			stateMu.Lock()
			cfg.ScrollSpeed = val
			stateMu.Unlock()
			log.Printf("ScrollSpeed set to %d", val)
		}

		chromedp.Run(ctx, chromedp.Reload())

		w.Write([]byte(fmt.Sprintf("Speed: %d", cfg.ScrollSpeed)))
	})

	// Config: URL
	http.HandleFunc("/config/url", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			return
		}
		newURL := r.URL.Query().Get("value")
		if newURL != "" {
			stateMu.Lock()
			cfg.TargetURL = newURL
			stateMu.Unlock()

			// Use Navigate, not Reload
			chromedp.Run(ctx, chromedp.Navigate(newURL))
			log.Printf("Target URL set to %s", newURL)
		}
		w.Write([]byte(fmt.Sprintf("URL: %s", cfg.TargetURL)))
	})

	// Reset
	http.HandleFunc("/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			return
		}

		// Reload fresh config from ENV (ignores current runtime changes)
		newCfg := LoadConfig()

		stateMu.Lock()
		cfg = newCfg
		stateMu.Unlock()

		// Apply all defaults
		go func() {
			// 1. Resize & Scale
			logicalW := int64(float64(cfg.Width) / cfg.ScaleFactor)
			logicalH := int64(float64(cfg.Height) / cfg.ScaleFactor)

			// Force separate actions to ensure they take effect
			chromedp.Run(ctx, emulation.SetDeviceMetricsOverride(logicalW, logicalH, cfg.ScaleFactor, false))
			chromedp.Run(ctx, chromedp.Navigate(cfg.TargetURL))
		}()

		log.Println("♻️  Configuration Reset to Defaults")
		w.Write([]byte("Reset Complete"))
	})

	// Config: Scale
	http.HandleFunc("/config/scale", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			return
		}
		val, _ := strconv.ParseFloat(r.URL.Query().Get("value"), 64)

		// Enforce Range 0.25 to 5.0
		if val < 0.25 {
			val = 0.25
		}
		if val > 5.0 {
			val = 5.0
		}

		stateMu.Lock()
		cfg.ScaleFactor = val
		// Recalculate based on current Width/Height
		logicalW := int64(float64(cfg.Width) / cfg.ScaleFactor)
		logicalH := int64(float64(cfg.Height) / cfg.ScaleFactor)
		stateMu.Unlock()

		// Apply immediately
		go func() {
			chromedp.Run(ctx,
				// Force Scale Factor 1.0 to avoid weird DPR artifacts
				// We rely purely on viewport resolution scaling
				emulation.SetDeviceMetricsOverride(logicalW, logicalH, 1.0, false),
				chromedp.Reload(),
			)
		}()
		log.Printf("ScaleFactor set to %f (Logical Viewport: %dx%d)", val, logicalW, logicalH)
		w.Write([]byte(fmt.Sprintf("Scale: %f", cfg.ScaleFactor)))
	})

	// Input Handler
	http.HandleFunc("/input", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			return
		}

		stateMu.RLock()
		locked := isLocked
		stateMu.RUnlock()

		if locked {
			http.Error(w, "Locked", http.StatusForbidden)
			return
		}

		var req struct {
			Type string  `json:"type"`
			X    float64 `json:"x"`
			Y    float64 `json:"y"`
			Text string  `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return
		}

		go func() {
			switch req.Type {
			case "click":
				// Adjust coordinates for ScaleFactor (Physical -> Logical)
				stateMu.RLock()
				sf := cfg.ScaleFactor
				stateMu.RUnlock()

				if sf <= 0 {
					sf = 1.0
				}

				logicalX := req.X / sf
				logicalY := req.Y / sf

				p1 := input.DispatchMouseEvent(input.MousePressed, logicalX, logicalY).WithButton("left").WithClickCount(1)
				p2 := input.DispatchMouseEvent(input.MouseReleased, logicalX, logicalY).WithButton("left").WithClickCount(1)
				chromedp.Run(ctx, p1, p2)
			case "type":
				dispatchSpecial := func(key string, code int64) error {
					return chromedp.Run(ctx,
						input.DispatchKeyEvent(input.KeyDown).WithNativeVirtualKeyCode(code).WithWindowsVirtualKeyCode(code).WithKey(key),
						input.DispatchKeyEvent(input.KeyUp).WithNativeVirtualKeyCode(code).WithWindowsVirtualKeyCode(code).WithKey(key),
					)
				}
				switch req.Text {
				case "Enter":
					dispatchSpecial("Enter", 13)
				case "Backspace":
					dispatchSpecial("Backspace", 8)
				case "ArrowUp":
					dispatchSpecial("ArrowUp", 38)
				case "ArrowDown":
					dispatchSpecial("ArrowDown", 40)
				case "ArrowLeft":
					dispatchSpecial("ArrowLeft", 37)
				case "ArrowRight":
					dispatchSpecial("ArrowRight", 39)
				case "PageUp":
					dispatchSpecial("PageUp", 33)
				case "PageDown":
					dispatchSpecial("PageDown", 34)
				default:
					if len(req.Text) == 1 {
						chromedp.Run(ctx,
							input.DispatchKeyEvent(input.KeyDown).WithText(req.Text).WithUnmodifiedText(req.Text),
							input.DispatchKeyEvent(input.KeyUp).WithText(req.Text).WithUnmodifiedText(req.Text),
						)
					}
				}
			}
		}()
		w.WriteHeader(http.StatusOK)
	})

	log.Printf("Listening on :%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, nil))
}

func startScreenshotLoop(ctx context.Context) {
	ticker := time.NewTicker(50 * time.Millisecond) // 20 FPS for smoother playback
	defer ticker.Stop()
	for range ticker.C {
		var buf []byte
		task := chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			buf, err = page.CaptureScreenshot().WithFormat(page.CaptureScreenshotFormatJpeg).WithQuality(70).Do(ctx)
			return err
		})
		if err := chromedp.Run(ctx, task); err == nil {
			imgMu.Lock()
			currentImg = buf
			imgMu.Unlock()
		}
	}
}

func startAutoScroll(ctx context.Context) {
	// Sync configuration to browser every 500ms
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	// Initial injection of the animation engine
	// We use a self-correcting loop that only scrolls if 'active' is true
	engineScript := `
		(function() {
			if (window._ctrlEngine) return; // Prevent double injection
			window._ctrlEngine = true;
			
			window._ctrl = { 
				active: false, 
				speed: 1.0, 
				state: 'top_pause', 
				ts: Date.now(),
				lastFrame: Date.now() 
			};

			function step() {
				// Use setInterval for robustness in headless
				// Calculate delta time to ensure constant speed
				const now = Date.now();
				const st = window._ctrl;
				const delta = (now - st.lastFrame) / 1000.0;
				st.lastFrame = now;

				if (!st.active) {
					return;
				}

				if (st.state === 'top_pause') {
					if (now - st.ts > 2000) { // 2s pause at top
						st.state = 'scrolling';
					}
				} else if (st.state === 'scrolling') {
					// Old Logic: speed / 3.0 per frame (16ms)
					// Matches approx: speed * 20 pixels/second
					// Delta logic: move = (speed * 20) * delta
					
					const move = (st.speed * 20) * delta;
					window.scrollBy(0, move);
					
					// check bottom (allow 2px error margin)
					if ((window.innerHeight + window.scrollY) >= document.body.scrollHeight - 2) {
						st.state = 'bottom_pause';
						st.ts = now;
					}
				} else if (st.state === 'bottom_pause') {
					if (now - st.ts > 2000) { // 2s pause at bottom
						window.scrollTo(0, 0);
						st.state = 'top_pause';
						st.ts = now;
					}
				}
			}
			setInterval(step, 16); // ~60 steps per second
		})();
	`

	// Try to inject engine periodically in case of reload
	go func() {
		injectTicker := time.NewTicker(2 * time.Second)
		defer injectTicker.Stop()
		for range injectTicker.C {
			chromedp.Run(ctx, chromedp.Evaluate(engineScript, nil))
		}
	}()

	for range ticker.C {
		stateMu.RLock()
		locked := isLocked
		shouldScroll := cfg.AutoScroll
		speed := float64(cfg.ScrollSpeed)
		stateMu.RUnlock()

		active := locked && shouldScroll

		// Sync state to JS
		updateScript := fmt.Sprintf(`
			if (window._ctrl) {
				window._ctrl.active = %v;
				window._ctrl.speed = %f;
			}
		`, active, speed)

		chromedp.Run(ctx, chromedp.Evaluate(updateScript, nil))
	}
}

func startAutoReload(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	lastReload := time.Now()

	for range ticker.C {
		stateMu.RLock()
		enabled := cfg.AutoReload
		interval := cfg.ReloadInt
		stateMu.RUnlock()

		if enabled && time.Since(lastReload) >= time.Duration(interval)*time.Second {
			log.Printf("🔄 Auto-Reloading (Interval: %ds)", interval)
			if err := chromedp.Run(ctx, chromedp.Reload()); err != nil {
				log.Printf("❌ Auto-Reload failed: %v", err)
			}
			lastReload = time.Now()
		}
	}
}

// Simple HTML Viewer (unchanged logic, refactored for brevity in this replace)
func serveViewer(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html><html><head><style>
  body, html { margin: 0; padding: 0; width: 100%; height: 100%; background: #000; overflow: hidden; }
  #stream { width: 100%; height: 100%; object-fit: fill; display: block; cursor: crosshair; }
  .click-indicator { position: absolute; width: 20px; height: 20px; background: rgba(255, 0, 0, 0.5); border-radius: 50%; transform: translate(-50%, -50%); pointer-events: none; animation: fadeOut 0.5s forwards; }
  @keyframes fadeOut { to { opacity: 0; transform: translate(-50%, -50%) scale(2); } }
</style></head><body><img id="stream" src="/stream" /><script>
    const img = document.getElementById('stream');
    function showClick(x, y) {
      const el = document.createElement('div'); el.className = 'click-indicator'; el.style.left = x + 'px'; el.style.top = y + 'px';
      document.body.appendChild(el); setTimeout(() => el.remove(), 500);
    }
    img.addEventListener('click', e => {
        const rect = img.getBoundingClientRect();
        // Use naturalWidth/Height to map client click to actual image coordinates
        const scaleX = img.naturalWidth / rect.width; 
        const scaleY = img.naturalHeight / rect.height;
        const x = (e.clientX - rect.left) * scaleX; 
        const y = (e.clientY - rect.top) * scaleY;
        showClick(e.clientX, e.clientY);
        fetch('/input', { method: 'POST', body: JSON.stringify({type: 'click', x: x, y: y}) });
    });
    document.addEventListener('keydown', e => {
        const isControl = e.key === "Backspace" || e.key === "Enter";
        const isNav = e.key.startsWith("Arrow") || e.key.startsWith("Page");
        const isChar = e.key.length === 1;
        if (isControl || isNav || isChar) {
             fetch('/input', { method: 'POST', body: JSON.stringify({type: 'type', text: e.key}) });
        }
    });
    window.onload = () => window.focus(); window.onclick = () => window.focus();
</script></body></html>`
	w.Write([]byte(html))
}

func serveStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=frame")
	for {
		imgMu.RLock()
		data := currentImg
		imgMu.RUnlock()
		if len(data) == 0 {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		fmt.Fprintf(w, "--frame\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", len(data))
		w.Write(data)
		w.Write([]byte("\r\n"))
		time.Sleep(100 * time.Millisecond)
	}
}

func ensureCtrlScript(dir string) {
	scriptPath := filepath.Join(dir, "ctrl.sh")
	content := `#!/bin/bash

# Load .env safely (ignoring comments)
if [ -f .env ]; then
    export $(grep -v '^#' .env | xargs)
fi

if [ -z "$CTRL_PORT" ]; then
    echo "Error: CTRL_PORT not found. Please ensure .env exists and contains CTRL_PORT."
    exit 1
fi

BASE_URL="http://localhost:$CTRL_PORT"
LAST_MSG=""

function show_help {
    echo "🎮 CTRL Master Control Script"
    echo "Usage: ./ctrl.sh [command] [args]"
    echo ""
    echo "Commands:"
    echo "  status               Show current configuration"
    echo "  lock                 🔒 Lock interaction"
    echo "  unlock               🔓 Unlock interaction"
    echo "  reload [on|off|int]  🔄 Reload page (on/off: auto-reload, int: interval)"
    echo "  url [url]            🌍 Set target URL"
    echo "  reset                ♻️  Reset to default configuration"
    echo "  autoscroll [on|off]  📜 Toggle auto-scroll"
    echo "  speed [val]          ⚡ Set scroll speed"
    echo "  scale [val]          🔍 Set zoom scale"
    echo "  q                    ❌ Quit interactive mode"
    echo ""
}

function run_cmd {
    local CMD=$1
    local ARG=$2
    
    case "$CMD" in
        status)
            echo "--- Current Status ---"
            # Parse JSON to multiline key: value
            curl -s "$BASE_URL/status" | sed -e 's/[{}]/''/g' -e 's/,"/\n/g' -e 's/"//g' -e 's/:/: /g'
            ;;
        lock)
            curl -X POST -s "$BASE_URL/lock"
            echo " -> Interaction Locked"
            ;;
        unlock)
            curl -X POST -s "$BASE_URL/unlock"
            echo " -> Interaction Unlocked"
            ;;
        reload)
            if [ -z "$ARG" ]; then
                curl -X POST -s "$BASE_URL/reload"
                echo " -> Page Reloaded"
            elif [ "$ARG" == "on" ]; then
                 curl -X POST -s "$BASE_URL/reload?state=on"
                 echo " -> Auto-Reload Enabled"
            elif [ "$ARG" == "off" ]; then
                 curl -X POST -s "$BASE_URL/reload?state=off"
                 echo " -> Auto-Reload Disabled"
            else
                 # Assume Integer
                 curl -X POST -s "$BASE_URL/reload?interval=$ARG"
                 echo " -> Reload Interval set to $ARG seconds"
            fi
            ;;
        url)
            if [ -n "$ARG" ]; then
                curl -X POST -s "$BASE_URL/config/url?value=$ARG"
                echo " -> URL set to $ARG"
            else
                echo "Usage: url [url]"
            fi
            ;;
        reset)
            if [ "$INTERACTIVE" == "1" ]; then
                read -p "⚠️  Are you sure you want to reset all settings to defaults? (y/N) " CONFIRM
                if [[ "$CONFIRM" =~ ^[Yy]$ ]]; then
                    curl -X POST -s "$BASE_URL/reset"
                    echo " -> Reset Complete"
                else
                    echo " -> Reset Cancelled"
                fi
            else
                 curl -X POST -s "$BASE_URL/reset"
                 echo " -> Reset Complete"
            fi
            ;;
        autoscroll)
            if [ "$ARG" == "on" ]; then
                curl -X POST -s "$BASE_URL/config/autoscroll?enabled=true"
                echo " -> AutoScroll Enabled"
            elif [ "$ARG" == "off" ]; then
                curl -X POST -s "$BASE_URL/config/autoscroll?enabled=false"
                echo " -> AutoScroll Disabled"
            else
                echo "Usage: autoscroll [on|off]"
            fi
            ;;
        speed)
            if [ -n "$ARG" ]; then
                curl -X POST -s "$BASE_URL/config/speed?value=$ARG"
                echo " -> Speed set to $ARG"
            else
                echo "Usage: speed [value]"
            fi
            ;;
        scale)
            if [ -n "$ARG" ]; then
                curl -X POST -s "$BASE_URL/config/scale?value=$ARG"
                echo " -> Scale set to $ARG"
            else
                echo "Usage: scale [value]"
            fi
            ;;
        q|quit|exit)
            exit 0
            ;;
        "")
            ;;
        *)
            echo "Unknown command: $CMD"
            ;;
    esac
}

# Interactive Mode
if [ -z "$1" ]; then
    INTERACTIVE=1
    while true; do
        clear
        show_help
        if [ -n "$LAST_MSG" ]; then
            echo -e "✅ $LAST_MSG\n"
        fi
        
        read -p "Enter command > " INPUT_CMD INPUT_ARG
        
        if [[ "$INPUT_CMD" == "q" ]]; then
            echo "Bye!"
            exit 0
        fi
        
        # Capture output
        LAST_MSG=$(run_cmd "$INPUT_CMD" "$INPUT_ARG")
    done
else
    # One-shot mode
    run_cmd "$1" "$2"
fi
`

	if err := os.WriteFile(scriptPath, []byte(content), 0755); err != nil {
		log.Printf("Error creating ctrl.sh: %v", err)
	} else {
		log.Printf("✅ Generated ctrl.sh at %s", scriptPath)
	}
}

func cleanupSingletonLock(dataDir string) {
	locks := []string{"SingletonLock", "SingletonCookie", "SingletonSocket"}
	for _, name := range locks {
		path := filepath.Join(dataDir, name)
		if _, err := os.Stat(path); err == nil {
			if err := os.Remove(path); err == nil {
				log.Printf("⚠️  Removed stale Chrome Lock file: %s", path)
			} else {
				log.Printf("❌ Failed to remove lock file %s: %v", path, err)
			}
		}
	}
}
