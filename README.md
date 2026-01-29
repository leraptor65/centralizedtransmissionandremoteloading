# Centralized Transmission and Remote Loading (CTRL)

A proxy to scale web pages for display, useful for displaying content on screens with different resolutions or scaling requirements.

## Deployment

### Docker Hub (Recommended)

You can easily deploy this using Docker Compose.

1.  **Download the compose file:**
    ```bash
    wget https://raw.githubusercontent.com/leraptor65/centralizedtransmissionandremoteloading/main/compose.yml
    ```

2.  **Run it:**
    ```bash
    # The configuration files (target_url.txt, etc.) will be created in the current directory
    # Ensure the container (UID 1000) can write to the current directory if permission issues arise.
    docker compose up -d
    ```

3.  **Configuration:**
    The application runs on port `1337` by default.
    You can configure the target URL and other settings by setting environment variables in the `compose.yml` file or by accessing the `/config` page (e.g., `http://localhost:1337/config`).

    **Environment Variables:**
    - `TARGET_URL`: The URL to proxy (default: `https://github.com/leraptor65/centralizedtransmissionandremoteloading`)
    - `SCALE_FACTOR`: Initial scale factor (default: `1.0`)

### Local Development / Manual Build

If you want to build the image locally instead of pulling from Docker Hub:

1.  Clone the repository.
2.  Build and run using Docker Compose:
    ```bash
    # Build the image and start the container
    docker compose -f docker-compose.dev.yml up -d --build
    ```
3.  Access the app at `http://localhost:1337`.

## Features

* **URL Display**: Show any public webpage through the proxy.
* **Custom Scaling**: Zoom in or out of any webpage to make it fit your display perfectly.
* **Advanced Auto-Scrolling**:
    * Enable or disable auto-scrolling with a single click.
    * Control the scroll speed in pixels per second.
    * Define custom scroll sequences (e.g., `0-500, 1200-1800`) to focus on specific content sections.
* **Live Page Height Reporting**: Automatically detects and displays the total pixel height of the target page to help you configure scroll sequences.
* **Live Reload**: The main display page automatically reloads when you save a new configuration.
* **Persistent Configuration**: Settings are saved to a Docker volume and persist even if the container is restarted or recreated.
* **High Compatibility**: Fixes common proxying issues by handling CORS, cookies, redirects, and security headers.

## Configuration Details
Go to `/config` to set:
- **Target URL**: The website you want to display.
- **Scale Factor**: How much to zoom/scale the page (e.g., `0.75` for 75%).
- **Auto Scroll**: Enable auto-scrolling.
- **Scroll Speed**: Speed of the scroll.
- **Scroll Sequence**: Advanced scrolling patterns.

## Built With

* [Node.js](https://nodejs.org/) - JavaScript runtime environment
* [Express.js](https://expressjs.com/) - Web framework for Node.js
* [Axios](https://axios-http.com/) - Promise-based HTTP client
* [Docker](https://www.docker.com/) - Containerization platform

## Author

* **leraptor65**

## License

This project is licensed under the MIT License - see the `LICENSE.md` file for details.
