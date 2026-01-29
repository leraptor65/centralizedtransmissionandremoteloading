package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/andybalholm/brotli"
)

// Regexes
var (
	cssUrlRe      = regexp.MustCompile(`(?i)url\(\s*(?:'([^']*)'|"([^"]*)"|([^'"\)]*))\s*\)`)
	htmlAttrRe    = regexp.MustCompile(`(?i)(href|src|action|poster)=('|")([^'"]*)('|")`)
	srcsetRe      = regexp.MustCompile(`(?i)srcset=('|")([^'"]*)('|")`)
	absoluteUrlRe = regexp.MustCompile(`('|")(https?:)?//([^/'"]+)`)
	importRe      = regexp.MustCompile(`(?i)@import\s+(?:url\()?["']?([^"'\)]+)["']?\)?[^;]*;`)
	integrityRe   = regexp.MustCompile(`(?i)\s*integrity="[^"]*"`)
	crossoriginRe = regexp.MustCompile(`(?i)\s*crossorigin(="[^"]*")?`)
)

func newProxyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		config := GetConfig()
		targetBase, err := url.Parse(config.TargetURL)
		if err != nil {
			http.Error(w, "Invalid Target URL", http.StatusInternalServerError)
			return
		}

		// URL Masking Logic:
		// Map localhost:1337/path -> TargetHost/path
		// If TargetBase has a path (e.g. /repo/bar), we prepend it if we are at root in this proxy.
		// However, simpler is usually better: just pass the path through.
		targetURL := *targetBase // Copy
		targetURL.Path = r.URL.Path
		targetURL.RawQuery = r.URL.RawQuery

		// Handle Root specifically if target has a subpath
		if r.URL.Path == "/" && targetBase.Path != "" && targetBase.Path != "/" {
			targetURL.Path = targetBase.Path
		}

		proxy := httputil.NewSingleHostReverseProxy(&targetURL)

		proxy.Director = func(req *http.Request) {
			req.Host = targetBase.Host
			req.URL.Scheme = targetBase.Scheme
			req.URL.Host = targetBase.Host
			req.URL.Path = targetURL.Path
			req.URL.RawQuery = targetURL.RawQuery

			req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
			req.Header.Set("Referer", fmt.Sprintf("%s://%s/", targetBase.Scheme, targetBase.Host))
			req.Header.Set("Origin", fmt.Sprintf("%s://%s", targetBase.Scheme, targetBase.Host))

			req.Header.Del("X-Forwarded-For")
			req.Header.Del("X-Real-IP")

			// Inject Cookies
			currentConfig := GetConfig()
			for _, c := range currentConfig.CookieJar {
				req.AddCookie(&http.Cookie{Name: c.Name, Value: c.Value})
			}

			req.Header.Del("Accept-Encoding")
			req.Header.Del("Connection")
		}

		proxy.ModifyResponse = func(resp *http.Response) error {
			// Cookies
			cookies := resp.Cookies()
			if len(cookies) > 0 {
				go UpdateCookies(cookies)
			}

			// Redirect Masking
			if resp.StatusCode >= 300 && resp.StatusCode < 400 {
				loc := resp.Header.Get("Location")
				if loc != "" {
					u, err := url.Parse(loc)
					if err == nil {
						abs := targetBase.ResolveReference(u)
						// If it's the same host, stay at localhost:1337
						if abs.Host == targetBase.Host {
							newPath := abs.Path
							if abs.RawQuery != "" {
								newPath += "?" + abs.RawQuery
							}
							resp.Header.Set("Location", newPath)
						} else {
							// Otherwise allow browser to follow to external host
							resp.Header.Set("Location", abs.String())
						}
					}
				}
			}

			resp.Header.Del("Content-Security-Policy")
			resp.Header.Del("Content-Security-Policy-Report-Only")
			resp.Header.Del("X-Frame-Options")

			contentType := resp.Header.Get("Content-Type")
			isText := strings.Contains(contentType, "text/html") ||
				strings.Contains(contentType, "text/css") ||
				strings.Contains(contentType, "javascript")

			if isText && resp.StatusCode == 200 {
				var reader io.ReadCloser
				var err error

				switch resp.Header.Get("Content-Encoding") {
				case "gzip":
					reader, err = gzip.NewReader(resp.Body)
					resp.Header.Del("Content-Encoding")
				case "br":
					reader = io.NopCloser(brotli.NewReader(resp.Body))
					resp.Header.Del("Content-Encoding")
				default:
					reader = resp.Body
				}

				if err != nil {
					return nil
				}

				bodyBytes, _ := io.ReadAll(reader)
				reader.Close()
				bodyStr := string(bodyBytes)

				// REWRITE LOGIC
				rewrite := func(u string) string {
					if u == "" || strings.HasPrefix(u, "data:") || strings.HasPrefix(u, "#") || strings.HasPrefix(u, "mailto:") {
						return u
					}
					ref, err := url.Parse(u)
					if err != nil {
						return u
					}
					abs := targetBase.ResolveReference(ref)

					// Masking:
					// If it's our target host, make it relative to root
					if abs.Host == targetBase.Host {
						newURL := abs.Path
						if abs.RawQuery != "" {
							newURL += "?" + abs.RawQuery
						}
						return newURL
					}

					// Otherwise, keep it absolute (no visible proxy prefix)
					return abs.String()
				}

				// Apply Rewrites
				bodyStr = cssUrlRe.ReplaceAllStringFunc(bodyStr, func(match string) string {
					sub := cssUrlRe.FindStringSubmatch(match)
					v := sub[1]
					if v == "" {
						v = sub[2]
					}
					if v == "" {
						v = sub[3]
					}
					if v == "" {
						return match
					}
					return fmt.Sprintf("url('%s')", rewrite(v))
				})

				bodyStr = importRe.ReplaceAllStringFunc(bodyStr, func(match string) string {
					sub := importRe.FindStringSubmatch(match)
					if len(sub) < 2 {
						return match
					}
					return strings.Replace(match, sub[1], rewrite(sub[1]), 1)
				})

				if strings.Contains(contentType, "text/html") {
					bodyStr = htmlAttrRe.ReplaceAllStringFunc(bodyStr, func(match string) string {
						sub := htmlAttrRe.FindStringSubmatch(match)
						return fmt.Sprintf("%s=%s%s%s", sub[1], sub[2], rewrite(sub[3]), sub[2])
					})

					bodyStr = srcsetRe.ReplaceAllStringFunc(bodyStr, func(match string) string {
						sub := srcsetRe.FindStringSubmatch(match)
						parts := strings.Split(sub[2], ",")
						for i, part := range parts {
							p := strings.TrimSpace(part)
							fields := strings.Fields(p)
							if len(fields) > 0 {
								fields[0] = rewrite(fields[0])
								parts[i] = strings.Join(fields, " ")
							}
						}
						return fmt.Sprintf("srcset=%s%s%s", sub[1], strings.Join(parts, ", "), sub[1])
					})

					bodyStr = integrityRe.ReplaceAllString(bodyStr, "")
					bodyStr = crossoriginRe.ReplaceAllString(bodyStr, "")

					// Inject Inventions
					type ClientConfig struct {
						AutoScroll     bool   `json:"autoScroll"`
						ScrollSpeed    int    `json:"scrollSpeed"`
						ScrollSequence string `json:"scrollSequence"`
					}
					clientConf := ClientConfig{
						AutoScroll:     config.AutoScroll,
						ScrollSpeed:    config.ScrollSpeed,
						ScrollSequence: config.ScrollSequence,
					}
					confBytes, _ := json.Marshal(clientConf)
					scripts := fmt.Sprintf(injectionsTemplate, string(confBytes), config.LastModified, config.ScaleFactor, 100.0/config.ScaleFactor)
					bodyStr = strings.Replace(bodyStr, "</head>", scripts+"</head>", 1)
				}

				buf := bytes.NewBufferString(bodyStr)
				resp.Body = io.NopCloser(buf)
				resp.Header.Set("Content-Length", strconv.Itoa(buf.Len()))
				resp.Header.Del("Transfer-Encoding")
			}
			return nil
		}

		proxy.ServeHTTP(w, r)
	}
}

func isBlocked(val string) bool {
	blocked := []string{"google-analytics.com", "googletagmanager.com", "doubleclick.net", "pagead2.googlesyndication.com"}
	for _, b := range blocked {
		if strings.Contains(val, b) {
			return true
		}
	}
	return false
}

const injectionsTemplate = `
<script>
    const config = %s;
    const initialVersion = %d;
    
    // Auto-Reload Logic
    setInterval(() => {
        fetch('/api/version')
            .then(res => res.json())
            .then(data => {
                if (data.lastModified > initialVersion) {
                    window.location.reload();
                }
            })
            .catch(() => {});
    }, 5000);

    // Locking Logic
    if (config.interfaceLocked) {
        const overlay = document.createElement('div');
        overlay.style.cssText = 'position:fixed;top:0;left:0;width:100vw;height:100vh;z-index:2147483647;background:transparent;cursor:none;';
        document.body.appendChild(overlay);

        const blockEvent = (e) => {
            if (e.isTrusted) {
                e.preventDefault();
                e.stopPropagation();
                return false;
            }
        };

        ['click', 'mousedown', 'mouseup', 'contextmenu', 'keydown', 'keyup', 'keypress', 'touchstart', 'touchend'].forEach(evt => {
            window.addEventListener(evt, blockEvent, true);
        });
        document.documentElement.style.cursor = 'none';
    }

    if (config.autoScroll) {
        document.addEventListener('DOMContentLoaded', () => {
            let lastTime = 0, currentSequenceIndex = 0, sequences = [], pauseUntil = 0;
            const PAUSE_DURATION_MS = 3000;
            function parseSequences() {
                const pageHeight = document.documentElement.scrollHeight - window.innerHeight;
                if (!config.scrollSequence.trim()) sequences.push({ start: 0, end: pageHeight });
                else {
                    sequences = config.scrollSequence.split(',').map(s => s.trim().split('-').map(Number)).filter(p => p.length === 2 && !isNaN(p[0]) && !isNaN(p[1])).map(p => ({ start: p[0], end: Math.min(p[1], pageHeight) }));
                    if (sequences.length === 0) sequences.push({ start: 0, end: pageHeight });
                }
            }
            function scrollStep(timestamp) {
                if (!lastTime) lastTime = timestamp;
                const deltaTime = timestamp - lastTime;
                lastTime = timestamp;
                if (Date.now() < pauseUntil) { requestAnimationFrame(scrollStep); return; }
                const current = sequences[currentSequenceIndex];
                window.scrollBy(0, (config.scrollSpeed / 1000) * deltaTime);
                if (window.scrollY >= current.end) {
                    currentSequenceIndex = (currentSequenceIndex + 1) % sequences.length;
                    window.scrollTo(0, sequences[currentSequenceIndex].start);
                    pauseUntil = Date.now() + PAUSE_DURATION_MS;
                }
                requestAnimationFrame(scrollStep);
            }
            parseSequences();
            if (sequences.length > 0) { window.scrollTo(0, sequences[0].start); requestAnimationFrame(scrollStep); }
        });
    }
    // Report height
    window.addEventListener('load', () => setTimeout(() => fetch('/api/report-height', { method: 'POST', body: JSON.stringify({height: document.documentElement.scrollHeight}) }), 2000));
</script>
<style>body{transform:scale(%.2f);transform-origin:0 0;width:%.2f%%;overflow-x:hidden;}</style>
`
