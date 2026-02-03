#!/bin/bash

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
