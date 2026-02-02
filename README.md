# C.T.R.L. (Centralized Transmission and Remote Loading)

A lightweight, headless Chrome automation tool driven by a simple HTTP API. Designed for displaying web dashboards, digital signage, or kiosks with remote control capabilities.

## 🚀 Quick Start

You don't need to clone the repository to run this. Just grab the `compose.yml` file.

1.  **Download `compose.yml`**
    Create a file named `compose.yml` with the following content (or download it from this repository):

    ```yaml
    services:
      ctrl:
        image: leraptor65/centralizedtransmissionandremoteloading:latest
        container_name: ctrl_app
        ports:
          - "1337:1337" # Host:Container (Can change Host port, e.g., "8080:1337")
        environment:
          - TARGET_URL=https://github.com/leraptor65
          - SCALE_FACTOR=1.0
          - AUTO_SCROLL=false
          - SCROLL_SPEED=10
        volumes:
          - ./data:/app/data
        security_opt:
          - seccomp=unconfined
        restart: unless-stopped
    ```

2.  **Run the Container**
    ```bash
    docker compose up -d
    ```

3.  **Control the Display**
    Upon startup, the container **automatically generates a control script** in your data folder.

    You can access the stream viewer at: `http://localhost:1337` (or your defined host port).

## 🛠️ Configuration

| Environment Variable | Default | Description |
| -------------------- | ------- | ----------- |
| `TARGET_URL` | (GitHub) | The URL to display on load. |
| `SCALE_FACTOR` | `1.0` | Zoom level (e.g. `1.5` for 150%). |
| `AUTO_SCROLL` | `false` | Enable/Disable auto-scroll loops. |
| `SCROLL_SPEED` | `10` | Pixels per step for auto-scroll. |

## 🏗️ Development & Building

If you wish to modify the code, you can build the project manually.

**Prerequisites:**
*   Docker
*   Go 1.23+ (optional, for local run without Docker)

**Build Helper:**
This repository includes helper scripts `build.sh` and `publish.sh` for development convenience. **Note:** These scripts are excluded from the repository and Docker Hub images to prevent accidental publishing.

**Manual Build:**

1.  Clone the repository.
2.  Build the Docker image:
    ```bash
    docker build -t ctrl-local .
    ```
3.  Run your local build:
    ```bash
    docker run -d -p 1337:1337 -v $(pwd)/data:/app/data --name ctrl-dev ctrl-local
    ```

## 📝 Notes

*   **`ctrl.sh`**: This script is generated inside the `/app/data` volume (mapped to `./data` on your host) every time the container starts. This ensures you always have the control script matching your current version.
*   **Security**: The container runs with `seccomp=unconfined` to allow Chrome to run properly in headless mode.

