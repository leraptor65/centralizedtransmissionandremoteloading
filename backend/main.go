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

	// Ensure ctrl.sh exists in data dir
	ensureCtrlScript(cfg.DataDir)

	// 1. Setup Chromedp
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("user-data-dir", cfg.DataDir),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	// 2. Initialize Browser
	if err := chromedp.Run(ctx,
		chromedp.Navigate(cfg.TargetURL),
		emulation.SetDeviceMetricsOverride(1920, 1080, cfg.ScaleFactor, false),
	); err != nil {
		log.Fatalf("Failed to start browser: %v", err)
	}

	// 3. Start Background Tasks
	go startScreenshotLoop(ctx)
	go startAutoScroll(ctx) // Always start, logic handled inside

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
		})
	})

	// Reload
	http.HandleFunc("/reload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			return
		}
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
		w.Write([]byte(fmt.Sprintf("Speed: %d", cfg.ScrollSpeed)))
	})

	// Config: Scale
	http.HandleFunc("/config/scale", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			return
		}
		val, _ := strconv.ParseFloat(r.URL.Query().Get("value"), 64)
		if val > 0 {
			stateMu.Lock()
			cfg.ScaleFactor = val
			stateMu.Unlock()
			// Apply immediately and RELOAD to ensure layout reflow updates
			go func() {
				chromedp.Run(ctx,
					emulation.SetDeviceMetricsOverride(1920, 1080, cfg.ScaleFactor, false),
					chromedp.Reload(),
				)
			}()
			log.Printf("ScaleFactor set to %f (Reloading...)", val)
		}
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
				p1 := input.DispatchMouseEvent(input.MousePressed, req.X, req.Y).WithButton("left").WithClickCount(1)
				p2 := input.DispatchMouseEvent(input.MouseReleased, req.X, req.Y).WithButton("left").WithClickCount(1)
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
	ticker := time.NewTicker(100 * time.Millisecond) // 10 FPS
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
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		stateMu.RLock()
		locked := isLocked
		shouldScroll := cfg.AutoScroll
		speed := cfg.ScrollSpeed
		stateMu.RUnlock()

		// Only scroll if Locked AND AutoScroll enabled
		if !locked || !shouldScroll {
			continue
		}

		// Scroll state machine (TopPause -> Scroll -> BottomPause -> Reset)
		script := fmt.Sprintf(`
			(function() {
				if (!window._ctrl) window._ctrl = { state: 'top_pause', ts: Date.now() };
				const now = Date.now();
				const st = window._ctrl;
				
				if (st.state === 'top_pause') {
					if (now - st.ts > 1000) {
						st.state = 'scrolling';
					}
				} else if (st.state === 'scrolling') {
					window.scrollBy(0, %d);
					if ((window.innerHeight + window.scrollY) >= document.body.scrollHeight - 1) {
						st.state = 'bottom_pause';
						st.ts = now;
					}
				} else if (st.state === 'bottom_pause') {
					if (now - st.ts > 1000) {
						window.scrollTo(0, 0);
						st.state = 'top_pause';
						st.ts = now;
					}
				}
			})();
		`, speed)
		chromedp.Run(ctx, chromedp.Evaluate(script, nil))
	}
}

// Simple HTML Viewer (unchanged logic, refactored for brevity in this replace)
func serveViewer(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html><html><head><style>
  body, html { margin: 0; padding: 0; width: 100%; height: 100%; background: #000; display: flex; justify-content: center; align-items: center; overflow: hidden; }
  #stream { max-width: 100%; max-height: 100%; box-shadow: 0 0 20px rgba(0,0,0,0.5); cursor: crosshair; }
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
        const scaleX = 1920 / rect.width; const scaleY = 1080 / rect.height;
        const x = (e.clientX - rect.left) * scaleX; const y = (e.clientY - rect.top) * scaleY;
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

BASE_URL="http://localhost:1337"

function show_help {
    echo "🎮 CTRL Master Control Script"
    echo "Usage: ./ctrl.sh [command] [args]"
    echo ""
    echo "Commands:"
    echo "  status               Show current configuration and lock state"
    echo "  lock                 🔒 Lock interaction (Enable AutoScroll if on)"
    echo "  unlock               🔓 Unlock interaction (Disable AutoScroll)"
    echo "  reload               🔄 Reload the browser page"
    echo "  autoscroll [on|off]  📜 Enable or Disable auto-scroll (Active when Locked)"
    echo "  speed [val]          ⚡ Set scroll speed (e.g., 10, 50)"
    echo "  scale [val]          🔍 Set zoom scale factor (e.g., 1.0, 1.5)"
    echo ""
}

if [ -z "$1" ]; then
    show_help
    exit 0
fi

CMD=$1

case "$CMD" in
    status)
        curl -s "$BASE_URL/status"
        ;;
    lock)
        curl -X POST "$BASE_URL/lock"
        echo ""
        ;;
    unlock)
        curl -X POST "$BASE_URL/unlock"
        echo ""
        ;;
    reload)
        curl -X POST "$BASE_URL/reload"
        echo ""
        ;;
    autoscroll)
        if [ "$2" == "on" ]; then
            curl -X POST "$BASE_URL/config/autoscroll?enabled=true"
        elif [ "$2" == "off" ]; then
            curl -X POST "$BASE_URL/config/autoscroll?enabled=false"
        else
            echo "Usage: ./ctrl.sh autoscroll [on|off]"
        fi
        echo ""
        ;;
    speed)
        if [ -n "$2" ]; then
            curl -X POST "$BASE_URL/config/speed?value=$2"
        else
            echo "Usage: ./ctrl.sh speed [value]"
        fi
        echo ""
        ;;
    scale)
        if [ -n "$2" ]; then
            curl -X POST "$BASE_URL/config/scale?value=$2"
        else
            echo "Usage: ./ctrl.sh scale [value]"
        fi
        echo ""
        ;;
    *)
        echo "Unknown command: $CMD"
        show_help
        ;;
esac
`

	if err := os.WriteFile(scriptPath, []byte(content), 0755); err != nil {
		log.Printf("Error creating ctrl.sh: %v", err)
	} else {
		log.Printf("✅ Generated ctrl.sh at %s", scriptPath)
	}
}
