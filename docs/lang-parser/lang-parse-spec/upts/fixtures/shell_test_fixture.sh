#!/bin/bash
# Shell Parser Test Fixture
# Tests symbol extraction for shell scripts
# Line numbers are predictable for UPTS validation

# === Pattern 1: Exported constants ===
# Line 8-12
export APP_NAME="mdemg-parser"
export APP_VERSION="1.0.0"
export DEBUG=true
export LOG_LEVEL="info"
export MAX_RETRIES=3

# === Pattern: Local variables (constants) ===
# Line 15-18
readonly CONFIG_FILE="/etc/app/config.yaml"
readonly DATA_DIR="/var/lib/app"
readonly LOG_DIR="/var/log/app"
readonly PID_FILE="/var/run/app.pid"

# === Pattern 2: Functions ===
# Line 21-24
log_info() {
    echo "[INFO] $(date '+%Y-%m-%d %H:%M:%S') $1"
}

# Line 26-29
log_error() {
    echo "[ERROR] $(date '+%Y-%m-%d %H:%M:%S') $1" >&2
}

# Line 31-36
check_dependencies() {
    local deps=("curl" "jq" "docker")
    for dep in "${deps[@]}"; do
        command -v "$dep" >/dev/null 2>&1 || { log_error "$dep not found"; exit 1; }
    done
}

# Line 38-44
setup_environment() {
    export PATH="/usr/local/bin:$PATH"
    export LANG="en_US.UTF-8"
    
    mkdir -p "$DATA_DIR" "$LOG_DIR"
    touch "$LOG_DIR/app.log"
}

# Line 46-56
start_service() {
    local config="${1:-$CONFIG_FILE}"
    local port="${2:-8080}"
    
    log_info "Starting service on port $port"
    
    if [[ -f "$PID_FILE" ]]; then
        log_error "Service already running"
        return 1
    fi
    
    ./app serve --config "$config" --port "$port" &
    echo $! > "$PID_FILE"
}

# Line 58-67
stop_service() {
    if [[ ! -f "$PID_FILE" ]]; then
        log_error "Service not running"
        return 1
    fi
    
    local pid
    pid=$(cat "$PID_FILE")
    kill "$pid" 2>/dev/null
    rm -f "$PID_FILE"
    log_info "Service stopped"
}

# Line 69-78
health_check() {
    local url="${1:-http://localhost:8080/health}"
    local timeout="${2:-5}"
    
    if curl -sf --max-time "$timeout" "$url" >/dev/null; then
        log_info "Health check passed"
        return 0
    else
        log_error "Health check failed"
        return 1
    fi
}

# === Pattern: Sourcing other files ===
# Line 81-83
if [[ -f "/etc/app/env.sh" ]]; then
    source "/etc/app/env.sh"
fi

# === Pattern: Main entry point ===
# Line 86-100
main() {
    local command="${1:-help}"
    
    case "$command" in
        start)
            check_dependencies
            setup_environment
            start_service "${@:2}"
            ;;
        stop)
            stop_service
            ;;
        health)
            health_check "${@:2}"
            ;;
        *)
            echo "Usage: $0 {start|stop|health}"
            exit 1
            ;;
    esac
}

# Line 102
main "$@"
