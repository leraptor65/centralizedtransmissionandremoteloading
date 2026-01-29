const express = require('express');
const axios = require('axios');
const fs = require('fs');
const path = require('path');
const yaml = require('js-yaml');
const { URL } = require('url');
const http = require('http');
const WebSocket = require('ws');
const cors = require('cors');
const zlib = require('zlib');
const cookie = require('cookie'); // For parsing and serializing cookies

const app = express();
const port = process.env.PORT || 1337;

// --- Middleware and Server Setup ---
app.use(cors());
const server = http.createServer(app);
const wss = new WebSocket.Server({ server });

// --- Live Reload Logic ---
wss.on('connection', ws => {
    console.log('Client connected for live reload.');
    ws.on('close', () => console.log('Client disconnected from live reload.'));
});

function broadcastReload() {
    console.log(`Broadcasting reload message to ${wss.clients.size} clients.`);
    wss.clients.forEach(client => {
        if (client.readyState === WebSocket.OPEN) client.send('reload');
    });
}

// --- Configuration Management ---
let lastReportedHeight = 'N/A';
// Map the host root (mounted at /usr/src/app/config_mount) as the data directory
const dataDir = path.join(__dirname, 'config_mount');
const settingsPath = path.join(dataDir, 'settings.yml');
const cookiePath = path.join(dataDir, 'cookies.json'); // Path for persistent cookies

// Default configuration
const defaultConfig = {
    targetUrl: 'https://github.com/leraptor65/centralizedtransmissionandremoteloading',
    scaleFactor: parseFloat(process.env.SCALE_FACTOR) || 1.0,
    autoScroll: process.env.AUTO_SCROLL === 'true',
    scrollSpeed: parseInt(process.env.SCROLL_SPEED) || 50,
    scrollSequence: process.env.SCROLL_SEQUENCE || '',
    history: [] // Array of { url: string, timestamp: number }
};

function getConfig() {
    let config = { ...defaultConfig };

    // Create defaults if settings.yml doesn't exist
    if (!fs.existsSync(settingsPath)) {
        if (!fs.existsSync(dataDir)) fs.mkdirSync(dataDir, { recursive: true });
        try {
            // Check for legacy files to migrate (best effort)
            const targetUrlPath = path.join(dataDir, 'target_url.txt');
            const configPath = path.join(dataDir, 'config.json');

            if (fs.existsSync(targetUrlPath)) {
                try {
                    config.targetUrl = fs.readFileSync(targetUrlPath, 'utf8').trim() || defaultConfig.targetUrl;
                } catch (e) { }
            }
            if (fs.existsSync(configPath)) {
                try {
                    const legacyConfig = JSON.parse(fs.readFileSync(configPath, 'utf8') || '{}');
                    config = { ...config, ...legacyConfig };
                } catch (e) { }
            }

            fs.writeFileSync(settingsPath, yaml.dump(config));
        } catch (e) {
            console.error("Error creating default settings.yml:", e);
        }
    } else {
        try {
            const rawData = fs.readFileSync(settingsPath, 'utf8');
            const loaded = yaml.load(rawData);
            config = { ...config, ...loaded };
        } catch (error) {
            console.error("Error reading settings.yml, falling back to defaults:", error);
        }
    }

    return config;
}

function saveConfig(newConfig) {
    if (!fs.existsSync(dataDir)) fs.mkdirSync(dataDir, { recursive: true });

    let currentConfig = getConfig();

    // Update history
    let history = currentConfig.history || [];
    if (newConfig.targetUrl && newConfig.targetUrl !== currentConfig.targetUrl) {
        // Remove if exists to bubble to top
        history = history.filter(item => item.url !== newConfig.targetUrl);
        history.unshift({ url: newConfig.targetUrl, timestamp: Date.now() });
        // Cap history at 20 items
        if (history.length > 20) history = history.slice(0, 20);
    }

    // Allow restoring explicit history from frontend save if needed, but usually we just append
    // If the frontend sends full history, use it? For now let's just append new URLs.

    const configToSave = {
        ...currentConfig,
        ...newConfig,
        history
    };

    try {
        fs.writeFileSync(settingsPath, yaml.dump(configToSave));
    } catch (e) {
        throw new Error(`Failed to write settings.yml: ${e.message}`);
    }
}

// --- Cookie Management Functions ---
function getCookies() {
    if (!fs.existsSync(cookiePath)) return {};
    try {
        const rawData = fs.readFileSync(cookiePath, 'utf8');
        return JSON.parse(rawData.trim() || '{}');
    } catch (error) {
        console.error("Error reading cookies.json:", error);
        return {};
    }
}

function saveCookies(cookies) {
    fs.writeFileSync(cookiePath, JSON.stringify(cookies, null, 2));
}

// --- Route Handlers ---

app.get('/favicon.ico', (req, res) => res.status(204).send());

app.post('/report-height', express.json(), (req, res) => {
    const { height } = req.body;
    if (height && typeof height === 'number') {
        lastReportedHeight = Math.round(height);
        console.log(`Received page height: ${lastReportedHeight}px`);
        res.status(200).send({ message: 'Height received' });
    } else {
        res.status(400).send({ message: 'Invalid height data' });
    }
});

app.get('/config', (req, res) => {
    const config = getConfig();
    fs.readFile(path.join(__dirname, 'config.html'), 'utf8', (err, html) => {
        if (err) return res.status(500).send('Error loading config page.');
        const finalHtml = html
            .replace('%%TARGET_URL%%', config.targetUrl)
            .replace('%%SCALE_FACTOR%%', config.scaleFactor)
            .replace('%%SCROLL_SPEED%%', config.scrollSpeed)
            .replace('%%SCROLL_SEQUENCE%%', config.scrollSequence || '')
            .replace('%%AUTOSCROLL_CHECKED%%', config.autoScroll ? 'checked' : '')
            .replace('%%PAGE_HEIGHT%%', lastReportedHeight)
            .replace('%%SUCCESS_CLASS%%', req.query.saved ? 'success' : '')
            .replace('%%HISTORY_ITEMS%%', JSON.stringify(config.history || []));
        res.send(finalHtml);
    });
});

app.post('/config', express.urlencoded({ extended: true }), (req, res) => {
    try {
        const { targetUrl, scaleFactor, scrollSpeed, scrollSequence } = req.body;
        saveConfig({ targetUrl, scaleFactor: parseFloat(scaleFactor), autoScroll: req.body.autoScroll === 'on', scrollSpeed: parseInt(scrollSpeed, 10), scrollSequence: scrollSequence || '' });
        broadcastReload();
        res.redirect('/config?saved=true');
    } catch (error) {
        console.error("!!! CRITICAL: Failed to save configuration:", error);
        res.status(500).send(`<h1>Error Saving Configuration</h1><p>Check container logs. Error: ${error.message}</p><a href="/config">Go back</a>`);
    }
});

app.post('/reset', (req, res) => {
    try {
        // Reset to defaults
        const defaults = { ...defaultConfig };
        // Keep existing history on reset? Or clear it? 
        // Let's clear history on full reset or provide separate options.
        // For now, full reset resets history too as per "defaults"

        // Actually, let's keep history if possible, or maybe user wants meaningful reset. 
        // Let's just reset config params but keep history?
        // "Reset to Default" implies factory reset. Let's reset everything.

        fs.writeFileSync(settingsPath, yaml.dump(defaults));
        broadcastReload();
        res.redirect('/config');
    } catch (error) {
        console.error("!!! CRITICAL: Failed to reset configuration:", error);
        res.status(500).send(`<h1>Error Resetting Configuration</h1><p>Check container logs. Error: ${error.message}</p><a href="/config">Go back</a>`);
    }
});

app.post('/clear-cookies', (req, res) => {
    try {
        if (fs.existsSync(cookiePath)) {
            fs.unlinkSync(cookiePath);
            console.log('Cleared saved cookies.');
        }
        broadcastReload(); // Reload to reflect logged-out state
        res.redirect('/config');
    } catch (error) {
        console.error("!!! CRITICAL: Failed to clear cookies:", error);
        res.status(500).send(`<h1>Error Clearing Cookies</h1><p>Check container logs. Error: ${error.message}</p><a href="/config">Go back</a>`);
    }
});

// --- Main Proxy Handler (MUST BE LAST) ---
app.use('/', async (req, res) => {
    const config = getConfig();
    const proxyHost = req.get('host');

    if (config.targetUrl.includes(proxyHost)) return res.status(500).send(`<h1>Configuration Error</h1><p>Target URL cannot be the proxy address. <a href="/config">Configure a different URL</a>.</p>`);
    let target;
    try {
        target = new URL(config.targetUrl);
    } catch (error) {
        return res.status(500).send(`<h1>Invalid Target URL</h1><p>URL "${config.targetUrl}" is not valid. <a href="/config">Please correct it</a>.</p>`);
    }

    let originalUrl = req.originalUrl;
    const protocol = req.headers['x-forwarded-proto'] || req.protocol;
    const proxyOrigin = `${protocol}://${proxyHost}`;

    const proxyHostPrefix = '/--proxy-host--/';
    if (originalUrl.startsWith(proxyHostPrefix)) {
        const parts = originalUrl.substring(proxyHostPrefix.length).split('/');
        const originalHost = parts.shift();
        // FIX: Normalize the path to prevent double slashes
        originalUrl = ('/' + parts.join('/')).replace(/\/+/g, '/');
        target = new URL(`${target.protocol}//${originalHost}`);
    }

    const targetUrl = originalUrl === '/' ? new URL(config.targetUrl) : new URL(originalUrl, target.origin);

    // FIX: Disable caching for the root redirect to ensure config changes are reflected immediately
    if (originalUrl === '/') {
        res.setHeader('Cache-Control', 'no-store, no-cache, must-revalidate, proxy-revalidate');
        res.setHeader('Pragma', 'no-cache');
        res.setHeader('Expires', '0');
        res.setHeader('Surrogate-Control', 'no-store');
    }

    // FIX: Block known ad/tracking domains to prevent anti-adblock detection triggered by failed requests.
    const blockList = [
        'pagead2.googlesyndication.com',
        'securepubads.g.doubleclick.net',
        'google-analytics.com',
        'googletagmanager.com',
        'adsbygoogle.js',
        'tpc.googlesyndication.com',
        'doubleclick.net',
        'fundingchoicesmessages.google.com',
        'cmp.quantcast.com'
    ];

    if (blockList.some(domain => targetUrl.hostname.includes(domain) || targetUrl.pathname.includes(domain))) {
        // Return a valid mock response so the client script thinks it loaded successfully.
        console.log(`Blocking ad/tracker request: ${targetUrl.href}`);
        if (targetUrl.pathname.endsWith('.js')) {
            res.setHeader('Content-Type', 'application/javascript');
            return res.send('// Blocked by proxy');
        } else if (targetUrl.pathname.endsWith('.css')) {
            res.setHeader('Content-Type', 'text/css');
            return res.send('/* Blocked by proxy */');
        } else {
            return res.status(200).send('');
        }
    }

    try {
        const storedCookies = getCookies();
        const browserCookies = req.headers.cookie ? cookie.parse(req.headers.cookie) : {};
        const combinedCookies = { ...storedCookies, ...browserCookies };
        const cookieHeader = Object.entries(combinedCookies).map(([key, value]) => `${key}=${value}`).join('; ');

        const response = await axios({
            method: req.method,
            url: targetUrl.href,
            headers: {
                ...req.headers,
                host: target.host,
                'Cookie': cookieHeader,
                // FIX: Add a standard User-Agent to improve compatibility
                'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/108.0.0.0 Safari/537.36',
            },
            responseType: 'stream',
            validateStatus: () => true,
            maxRedirects: 0, // We handle redirects manually
            data: (req.method !== 'GET' && req.method !== 'HEAD') ? req : undefined,
        });

        const setCookieHeader = response.headers['set-cookie'];
        if (setCookieHeader) {
            const newCookies = getCookies();
            setCookieHeader.forEach(cookieString => {
                const parts = cookie.parse(cookieString);
                const [name] = Object.keys(parts);
                if (parts[name]) newCookies[name] = parts[name];
            });
            saveCookies(newCookies);
            res.setHeader('Set-Cookie', setCookieHeader);
        }

        // FIX: Robust redirect handling for cross-domain logins
        if ([301, 302, 307, 308].includes(response.status)) {
            const locationHeader = response.headers.location;
            if (!locationHeader) {
                return res.status(response.status).send('Redirect with no location header');
            }
            const newLocation = new URL(locationHeader, targetUrl.origin);
            const proxiedRedirectUrl = `${proxyOrigin}${proxyHostPrefix}${newLocation.host}${newLocation.pathname}${newLocation.search}`;
            console.log(`Redirecting to: ${proxiedRedirectUrl}`);
            // FIX: Force temporary redirect (307) to prevent browser caching of the rewritten redirect
            // This ensures that if the target URL changes, the browser will re-evaluate '/' instead of using a cached redirect.
            return res.redirect(307, proxiedRedirectUrl);
        }

        Object.keys(response.headers).forEach(key => {
            const lowerKey = key.toLowerCase();
            if (!['content-security-policy', 'x-frame-options', 'transfer-encoding', 'content-encoding', 'content-length', 'set-cookie'].includes(lowerKey)) {
                res.setHeader(key, response.headers[key]);
            }
        });

        const contentType = response.headers['content-type'] || '';
        const contentEncoding = response.headers['content-encoding'];
        let stream = response.data;
        if (contentEncoding === 'gzip') stream = stream.pipe(zlib.createGunzip());
        else if (contentEncoding === 'deflate') stream = stream.pipe(zlib.createInflate());
        else if (contentEncoding === 'br') stream = stream.pipe(zlib.createBrotliDecompress());

        const validTypes = ['text/html', 'text/css', 'application/javascript', 'application/x-javascript', 'text/javascript'];
        let isText = validTypes.some(type => contentType.includes(type));

        if (isText) {
            const chunks = [];
            for await (const chunk of stream) chunks.push(chunk);
            let body = Buffer.concat(chunks).toString();

            // Helper to rewrite a single URL
            const rewriteUrl = (url) => {
                if (!url || url.startsWith('data:') || url.startsWith('#') || url.startsWith('mailto:')) return url;
                // Don't rewrite if already proxied (simple check)
                if (url.includes(proxyHostPrefix)) return url;

                try {
                    // Resolve relative against the current target URL
                    const resolved = new URL(url, targetUrl.href);
                    // Rewrite to proxy format
                    return `${proxyOrigin}${proxyHostPrefix}${resolved.host}${resolved.pathname}${resolved.search}`;
                } catch (e) {
                    return url; // Fallback
                }
            };

            // 1. CSS URL rewriting (url('...')) - Global replace
            body = body.replace(/url\(\s*(['"]?)(.*?)\1\s*\)/gi, (match, quote, url) => {
                return `url(${quote}${rewriteUrl(url)}${quote})`;
            });

            // 2. HTML Attribute rewriting (src, href, action, poster)
            if (contentType.includes('text/html')) {
                body = body.replace(/(href|src|action|poster)=(['"])(.*?)\2/gi, (match, attr, quote, url) => {
                    return `${attr}=${quote}${rewriteUrl(url)}${quote}`;
                });

                // 3. HTML srcset rewriting
                body = body.replace(/srcset=(['"])(.*?)\1/gi, (match, quote, srcset) => {
                    const newSrcset = srcset.split(',').map(entry => {
                        const parts = entry.trim().split(/\s+/);
                        const url = parts[0];
                        if (url) {
                            parts[0] = rewriteUrl(url);
                            return parts.join(' ');
                        }
                        return entry;
                    }).join(', ');
                    return `srcset=${quote}${newSrcset}${quote}`;
                });

                // 4. Integrity and crossorigin removal
                body = body.replace(/integrity="[^"]*"/gi, '').replace(/\s+crossorigin(="[^"]*")?/gi, '');

                // Inject Custom Scripts (only for HTML)
                const injectedScripts = `
                    <script>
                        const config = { autoScroll: ${config.autoScroll}, scrollSpeed: ${config.scrollSpeed}, scrollSequence: "${config.scrollSequence || ''}" };
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
                        window.addEventListener('load', () => setTimeout(() => fetch('/report-height', { method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({height: document.documentElement.scrollHeight}) }), 2000));
                        const socket = new WebSocket((window.location.protocol === 'https:' ? 'wss:' : 'ws:') + '//' + window.location.host);
                        socket.addEventListener('message', e => {
                            if (e.data === 'reload') {
                                console.log('Configuration changed. Redirecting to home...');
                                window.location.href = '/';
                            }
                        });
                    </script>`;
                const styling = `<style>body{transform:scale(${config.scaleFactor});transform-origin:0 0;width:${100 / config.scaleFactor}%;overflow-x:hidden;}</style>`;
                body = body.replace('</head>', `${styling}${injectedScripts}</head>`);
            }

            res.send(body);
        } else {
            res.status(response.status);
            stream.pipe(res);
        }
    } catch (error) {
        console.error(`Proxy error for ${targetUrl.href}:`, error.message);
        // FIX: Return 204 instead of 500 to prevent triggering client-side error handlers
        if (!res.headersSent) {
            res.status(204).send();
        }
    }
});

server.listen(port, () => {
    console.log(`Web Scaler Proxy running on http://localhost:${port}`);
});

