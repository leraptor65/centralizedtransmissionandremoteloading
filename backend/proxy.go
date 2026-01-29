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

const proxyHostPrefix = "/--proxy-host--/"

// Regexes (adapted for Go RE2 which doesn't support backreferences)
var (
	// url('...') or url("...") or url(...)
	cssUrlRe = regexp.MustCompile(`(?i)url\(\s*(?:'([^']*)'|"([^"]*)"|([^'"\)]*))\s*\)`)
	// href="...", src='...'
	htmlAttrRe = regexp.MustCompile(`(?i)(href|src|action|poster)=('|")([^'"]*)('|")`)
	// srcset="..."
	srcsetRe = regexp.MustCompile(`(?i)srcset=('|")([^'"]*)('|")`)
	// Absolute URLs: "//domain.com" or "https://domain.com" inside quotes
	absoluteUrlRe = regexp.MustCompile(`('|")(https?:)?//([^/'"]+)`)

	// @import "..." or @import url(...)
	importRe = regexp.MustCompile(`(?i)@import\s+(?:url\()?["']?([^"'\)]+)["']?\)?[^;]*;`)

	integrityRe   = regexp.MustCompile(`(?i)\s*integrity="[^"]*"`)
	crossoriginRe = regexp.MustCompile(`(?i)\s*crossorigin(="[^"]*")?`)
)

func newProxyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		config := GetConfig()
		targetURL, err := url.Parse(config.TargetURL)
		if err != nil {
			http.Error(w, "Invalid Target URL", http.StatusInternalServerError)
			return
		}

		// Handle Proxy Host Prefix
		// /--proxy-host--/host/path...
		proxyHost := r.Host
		proxyOrigin := fmt.Sprintf("http://%s", proxyHost) // Assuming http for internal -> external connection, or determining from request
		if r.TLS != nil {
			proxyOrigin = fmt.Sprintf("https://%s", proxyHost)
		}

		target := targetURL
		reqPath := r.URL.Path

		if strings.HasPrefix(reqPath, proxyHostPrefix) {
			pathParts := strings.Split(strings.TrimPrefix(reqPath, proxyHostPrefix), "/")
			if len(pathParts) > 0 {
				originalHost := pathParts[0]
				// path is everything after host
				newPath := "/" + strings.Join(pathParts[1:], "/")
				// Normalize path
				newPath = strings.ReplaceAll(newPath, "//", "/")

				// Construct new target
				target = &url.URL{
					Scheme: targetURL.Scheme,
					Host:   originalHost,
					Path:   newPath,
				}
				reqPath = newPath
			}
		} else if reqPath == "/" {
			// Root request
			// If the targetURL has a specific path (e.g. /leraptor65/...), and we are at root,
			// we should use that path, unless the user manually navigated to /
			// But for a mirror/proxy, accessing root usually implies accessing the target Root.
			// However, if TargetURL is `https://github.com/foo/bar`, accessing `localhost:1337/` should proxy `https://github.com/foo/bar`.
			// The current logic in `httputil.NewSingleHostReverseProxy` uses `target.Path` as a base, but we need to ensure we don't double up or lose it.
			// Actually, standard RecverseProxy behavior:
			// "If the target is https://example.com/base and the request is /dir, the request URL to the target will be https://example.com/base/dir."
			// So if we just set target to targetURL, it should work?
			// But we are manually setting `req.URL.Path = reqPath` in Director.
			// `reqPath` is `/`.
			// If we overwrite `req.URL.Path = reqPath`, we lose `targetURL.Path`.

			// Fix: If request is root, and target has path, let's allow the ReverseProxy's default behavior OR map explicitly.
			// If we want `localhost:1337/` -> `github.com/foo/bar`, we should set req.URL.Path to `/foo/bar`.
			if targetURL.Path != "" && targetURL.Path != "/" {
				reqPath = targetURL.Path
			}

			// Disable caching for root
			w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
		}

		// Create Check for Blocklist
		if isBlocked(target.Host) || isBlocked(target.Path) {
			if strings.HasSuffix(target.Path, ".js") {
				w.Header().Set("Content-Type", "application/javascript")
				w.Write([]byte("// Blocked by proxy"))
				return
			} else if strings.HasSuffix(target.Path, ".css") {
				w.Header().Set("Content-Type", "text/css")
				w.Write([]byte("/* Blocked by proxy */"))
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}

		proxy := httputil.NewSingleHostReverseProxy(target)

		// Update Director to properly set headers
		originalDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			originalDirector(req)
			req.Host = target.Host // Important: Set Host header to target
			req.URL.Path = reqPath
			req.URL.Host = target.Host
			req.URL.Scheme = target.Scheme

			// Spoof Headers for compatibility
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/108.0.0.0 Safari/537.36")
			req.Header.Set("Referer", fmt.Sprintf("%s://%s/", target.Scheme, target.Host))
			req.Header.Set("Origin", fmt.Sprintf("%s://%s", target.Scheme, target.Host))

			// Inject Shared Cookies
			// We iterate over our jar and add them
			currentConfig := GetConfig()
			for _, c := range currentConfig.CookieJar {
				// We add to the request
				req.AddCookie(&http.Cookie{
					Name:  c.Name,
					Value: c.Value,
				})
			}

			req.Header.Del("Accept-Encoding") // Let's request Identity to avoid manual decompression mess if possible
			// Actually, requesting compressed is fine if we handle it in ModifyResponse
		}

		proxy.ModifyResponse = func(resp *http.Response) error {
			// Capture Cookies
			cookies := resp.Cookies()
			if len(cookies) > 0 {
				go UpdateCookies(cookies) // Async update to not block
			}

			// Handle Redirects
			if resp.StatusCode >= 300 && resp.StatusCode < 400 {
				loc := resp.Header.Get("Location")
				if loc != "" {
					newLoc, err := url.Parse(loc)
					if err == nil {
						// Resolve relative
						newLoc = target.ResolveReference(newLoc)
						proxiedUrl := fmt.Sprintf("%s%s%s%s", proxyOrigin, proxyHostPrefix, newLoc.Host, newLoc.Path)
						if newLoc.RawQuery != "" {
							proxiedUrl += "?" + newLoc.RawQuery
						}
						resp.Header.Set("Location", proxiedUrl)
						resp.StatusCode = 307 // Force temp redirect
					}
				}
			}

			// Strip Security Headers to allow framing and proxying
			resp.Header.Del("Content-Security-Policy")
			resp.Header.Del("Content-Security-Policy-Report-Only")
			resp.Header.Del("X-Frame-Options")

			// Rewriting Logic
			contentType := resp.Header.Get("Content-Type")
			isText := strings.Contains(contentType, "text/html") ||
				strings.Contains(contentType, "text/css") ||
				strings.Contains(contentType, "javascript")

			if isText && resp.StatusCode == 200 {
				var reader io.ReadCloser
				var err error

				// Decompress if needed
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
					return nil // error reading body
				}

				bodyBytes, err := io.ReadAll(reader)
				reader.Close() // Close original reader
				if err != nil {
					return err
				}

				bodyStr := string(bodyBytes)

				// REWRITE FUNCTIONS
				rewrite := func(u string) string {
					if u == "" || strings.HasPrefix(u, "data:") || strings.HasPrefix(u, "#") || strings.HasPrefix(u, "mailto:") {
						return u
					}
					if strings.Contains(u, proxyHostPrefix) {
						return u
					}

					// Resolve
					ref, err := url.Parse(u)
					if err != nil {
						return u
					}
					abs := target.ResolveReference(ref)

					// If it's a different host, or absolute
					return fmt.Sprintf("%s%s%s%s", proxyOrigin, proxyHostPrefix, abs.Host, abs.Path) + func() string {
						if abs.RawQuery != "" {
							return "?" + abs.RawQuery
						}
						return ""
					}()
				}

				// 1. CSS URL & Import
				bodyStr = cssUrlRe.ReplaceAllStringFunc(bodyStr, func(match string) string {
					submatch := cssUrlRe.FindStringSubmatch(match)
					urlVal := submatch[1]
					if urlVal == "" {
						urlVal = submatch[2]
					}
					if urlVal == "" {
						urlVal = submatch[3]
					}
					if urlVal == "" {
						return match
					}
					return fmt.Sprintf("url('%s')", rewrite(urlVal))
				})

				bodyStr = importRe.ReplaceAllStringFunc(bodyStr, func(match string) string {
					sub := importRe.FindStringSubmatch(match)
					if len(sub) < 2 {
						return match
					}
					val := sub[1]
					return strings.Replace(match, val, rewrite(val), 1)
				})

				// 2. HTML Attributes
				if strings.Contains(contentType, "text/html") {
					bodyStr = htmlAttrRe.ReplaceAllStringFunc(bodyStr, func(match string) string {
						// groups: 1=attr, 2=quote, 3=val, 4=quote
						sub := htmlAttrRe.FindStringSubmatch(match)
						attr := sub[1]
						quote := sub[2]
						val := sub[3]
						return fmt.Sprintf("%s=%s%s%s", attr, quote, rewrite(val), quote)
					})

					// 3. Srcset
					bodyStr = srcsetRe.ReplaceAllStringFunc(bodyStr, func(match string) string {
						sub := srcsetRe.FindStringSubmatch(match)
						quote := sub[1]
						val := sub[2]

						// split comma
						parts := strings.Split(val, ",")
						for i, part := range parts {
							p := strings.TrimSpace(part)
							subParts := strings.Fields(p) // split by space
							if len(subParts) > 0 {
								subParts[0] = rewrite(subParts[0])
								parts[i] = strings.Join(subParts, " ")
							}
						}
						return fmt.Sprintf("srcset=%s%s%s", quote, strings.Join(parts, ", "), quote)
					})

					// 4. Cleanup
					bodyStr = integrityRe.ReplaceAllString(bodyStr, "")
					bodyStr = crossoriginRe.ReplaceAllString(bodyStr, "")

					// 5. Absolute URLs //domain.com
					// This is harder with regex without negative lookbehind for proxy prefix
					// Let's rely on basic replacement for obvious ones
					// Or skip simpler regex and trust the attribute parsers above
					// The nodeJS one had a generic replacer for " //..."
					// absoluteUrlRe = regexp.MustCompile(`('|")(https?:)?//([^/'"]+)`)
					bodyStr = absoluteUrlRe.ReplaceAllStringFunc(bodyStr, func(match string) string {
						sub := absoluteUrlRe.FindStringSubmatch(match)
						quote := sub[1]
						// sub[2] protocol, sub[3] host
						protocol := sub[2]
						if protocol == "" {
							protocol = "http:"
						} // assume http if //
						host := sub[3]
						if host == proxyHost {
							return match
						}

						return fmt.Sprintf("%s%s%s%s%s", quote, proxyOrigin, proxyHostPrefix, host, "")
						// Note: this regex is risky, might match text content.
						// Confined to quotes makes it safer but not perfect.
					})

					// INJECT SCRIPTS
					// Create config object
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

					scripts := fmt.Sprintf(injectionsTemplate, string(confBytes), config.LastModified, config.ScaleFactor, 100/config.ScaleFactor)
					bodyStr = strings.Replace(bodyStr, "</head>", scripts+"</head>", 1)
				}

				// Re-assign body
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

func isBlocked(host string) bool {
	// Add blocklist logic here
	blocked := []string{
		"pagead2.googlesyndication.com",
		"google-analytics.com",
		"googletagmanager.com",
		"doubleclick.net",
		"intercom.io",
	}
	for _, b := range blocked {
		if strings.Contains(host, b) {
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
                    console.log("Config changed, reloading...");
                    window.location.reload();
                }
            })
            .catch(err => console.error("Version check failed", err));
    }, 2000);

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
                    currentSequenceIndex = (currentSequenceIndex + 1) %% sequences.length;
                    window.scrollTo(0, sequences[currentSequenceIndex].start);
                    pauseUntil = Date.now() + PAUSE_DURATION_MS;
                }
                requestAnimationFrame(scrollStep);
            }
            parseSequences();
            if (sequences.length > 0) { window.scrollTo(0, sequences[0].start); requestAnimationFrame(scrollStep); }
        });
    }
    // Report height logic...
    window.addEventListener('load', () => setTimeout(() => fetch('/api/report-height', { method: 'POST', body: JSON.stringify({height: document.documentElement.scrollHeight}) }), 2000));
</script>
<style>body{transform:scale(%.2f);transform-origin:0 0;width:%.2f%%;overflow-x:hidden;}</style>
`
