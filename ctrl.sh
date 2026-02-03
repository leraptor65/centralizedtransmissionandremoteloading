#!/bin/bash

# Load .env if it exists
if [ -f .env ]; then
    export $(cat .env | xargs)
fi

PORT="${CTRL_PORT:-1337}"
BASE_URL="http://localhost:$PORT"

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
