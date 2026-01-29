# Centralized Transmission and Remote Loading (CTRL)

A lightweight proxy to scale and auto-scroll web pages for remote displays. Controlled entirely via environment variables.

## Deployment

### Docker (Recommended)

You can easily deploy this using Docker Compose.

1.  **Preparation:**
    Ensure you have a `.env` file in your directory.

2.  **Run it:**
    ```bash
    docker compose up -d
    ```

3.  **Configuration:**
    The application runs on port `1337` by default. Everything is configured via the `.env` file.

    **Environment Variables:**
    - `TARGET_URL`: The URL to proxy (e.g., `https://github.com/`)
    - `SCALE_FACTOR`: Initial scale factor (e.g., `1.2`)
    - `AUTO_SCROLL`: Enable auto-scrolling (`true`/`false`)
    - `SCROLL_SPEED`: Speed in pixels per second (e.g., `50`)
    - `SCROLL_SEQUENCE`: Custom scroll sections (e.g., `0-1000, 2000-3000`)

4.  **Persistent Data:**
    Cookies and session data are stored in a `./data` folder automatically created on the host. To reset the proxy state (clear cookies), simply delete this folder and restart the container.

### Local Development

1.  **Prerequisites:**
    *   Go 1.23+

2.  **Run the Backend:**
    ```bash
    cd backend
    go run .
    ```

3.  Access the app at `http://localhost:1337`.

## Features

* **URL Masking**: Stay on `localhost:1337` regardless of internal navigation.
* **Custom Scaling**: Precise control over page zoom.
* **Auto-Scrolling**: Automated movement through page sections.
* **Persistent Sessions**: Cookies are saved to disk and reused across restarts.
* **Zero UI**: Minimal footprint, purely driven by environment state.

## License

This project is licensed under the MIT License.
