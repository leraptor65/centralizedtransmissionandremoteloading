# C.T.R.L. (Centralized Transmission and Remote Loading)

A lightweight, headless Chrome automation tool driven by a simple HTTP API. Designed for displaying web dashboards, digital signage, or kiosks with remote control capabilities.

## 🚀 Quick Start

You don't need to clone the repository to run this. Just grab the files and go.

1.  **Download `compose.yml`**
    ```bash
    # using curl
    curl -o compose.yml https://raw.githubusercontent.com/leraptor65/centralizedtransmissionandremoteloading/master/compose.yml
    ```
    ```bash
    # OR using wget
    wget https://raw.githubusercontent.com/leraptor65/centralizedtransmissionandremoteloading/master/compose.yml
    ```

2.  **Configure Environment**
    Create a `.env` file in the same directory (copy-paste block):
    ```ini
    CTRL_PORT=1337
    TARGET_URL=https://github.com/leraptor65
    SCALE_FACTOR=1.0
    AUTO_SCROLL=false
    SCROLL_SPEED=10
    AUTO_RELOAD=false
    RELOAD_INTERVAL=60
    WIDTH=1920
    HEIGHT=1080
    DATA_DIR=./data
    SCRIPT_DIR=/host
    ```

3.  **Run**
    ```bash
    docker compose up -d
    ```

4.  **Control**
    A `ctrl.sh` script will be generated in your directory properly configured to talk to the container.
    ```bash
    ./ctrl.sh
    ```

## 🛠️ Configuration

| Environment Variable | Default | Description |
| -------------------- | ------- | ----------- |
| `CTRL_PORT` | `1337` | Port to access the viewer and API. |
| `TARGET_URL` | (GitHub) | The URL to display on load. |
| `SCALE_FACTOR` | `1.0` | Zoom level (e.g. `1.5` for 150%). |
| `AUTO_SCROLL` | `false` | Enable/Disable auto-scroll loops. |
| `SCROLL_SPEED` | `10` | Pixels per step for auto-scroll. |
| `AUTO_RELOAD` | `false` | Enable/Disable auto page reload. |
| `RELOAD_INTERVAL` | `60` | Interval in seconds for auto-reload. |
| `WIDTH` | `1920` | Browser viewport width. |
| `HEIGHT` | `1080` | Browser viewport height. |

## 🏗️ Development & Building

If you wish to modify the code, you can build the project manually.

**Prerequisites:**
*   Docker
*   Go 1.23+ (optional, for local run without Docker)



**Build:**

1.  Clone the repository.
    ```bash
    git clone https://github.com/leraptor65/centralizedtransmissionandremoteloading.git
    ```
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

