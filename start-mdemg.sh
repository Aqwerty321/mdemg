#!/bin/bash
# Start MDEMG Memory System for IDE Agent Integration
#
# Prerequisites:
#   - Docker running with Neo4j container
#   - Embedding provider: OpenAI (API key) or Ollama (local)
#
# Configuration:
#   Edit mdemg_build/service/.env to configure embedding provider
#
# Usage: ./start-mdemg.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVICE_DIR="$SCRIPT_DIR/mdemg_build/service"
ENV_FILE="$SERVICE_DIR/.env"

echo "=== MDEMG Memory System Startup ==="

# Source .env file if it exists
if [ -f "$ENV_FILE" ]; then
    echo "Loading configuration from .env..."
    set -a
    source "$ENV_FILE"
    set +a
else
    echo "WARNING: No .env file found at $ENV_FILE"
    echo "Using default configuration (Ollama)"
fi

# Check Neo4j
echo -n "Checking Neo4j... "
if docker ps --filter "name=mdemg-neo4j" --format "{{.Status}}" | grep -q "healthy"; then
    echo "OK (running)"
else
    echo "Starting Neo4j..."
    docker compose -f "$SCRIPT_DIR/docker-compose.yml" up -d
    echo "Waiting for Neo4j to be healthy..."
    sleep 10
fi

# Check embedding provider
if [ "$EMBEDDING_PROVIDER" = "ollama" ] || [ -z "$EMBEDDING_PROVIDER" ]; then
    echo -n "Checking Ollama... "
    if curl -s http://localhost:11434/api/tags >/dev/null 2>&1; then
        echo "OK (running)"
    else
        echo "WARNING: Ollama not running. Start with: ollama serve"
    fi

    # Check embedding model
    OLLAMA_MODEL="${OLLAMA_MODEL:-nomic-embed-text}"
    echo -n "Checking embedding model... "
    if curl -s http://localhost:11434/api/tags 2>/dev/null | grep -q "$OLLAMA_MODEL"; then
        echo "OK ($OLLAMA_MODEL)"
    else
        echo "Pulling $OLLAMA_MODEL model..."
        ollama pull "$OLLAMA_MODEL"
    fi
elif [ "$EMBEDDING_PROVIDER" = "openai" ]; then
    echo -n "Checking OpenAI... "
    if [ -n "$OPENAI_API_KEY" ]; then
        echo "OK (API key configured)"
    else
        echo "ERROR: OPENAI_API_KEY not set in .env"
        exit 1
    fi
fi

# Build and start MDEMG service
echo "Building MDEMG service..."
cd "$SERVICE_DIR"
go build -o /tmp/mdemg-server ./cmd/server

# Set defaults for required variables if not in .env
export NEO4J_URI="${NEO4J_URI:-bolt://localhost:7687}"
export NEO4J_USER="${NEO4J_USER:-neo4j}"
export NEO4J_PASS="${NEO4J_PASS:-testpassword}"
export REQUIRED_SCHEMA_VERSION="${REQUIRED_SCHEMA_VERSION:-4}"
export LISTEN_ADDR="${LISTEN_ADDR:-:8082}"

echo "Starting MDEMG service on $LISTEN_ADDR..."
echo "Embedding provider: ${EMBEDDING_PROVIDER:-ollama}"

/tmp/mdemg-server &
MDEMG_PID=$!

sleep 2

# Verify service is running
if curl -s http://localhost:8082/readyz | grep -q "ready"; then
    echo ""
    echo "=== MDEMG Memory System Ready ==="
    echo "Service PID: $MDEMG_PID"
    echo "API Endpoint: http://localhost:8082"
    echo "Neo4j Browser: http://localhost:7474"
    echo ""
    echo "MCP Server: mdemg_build/mcp-server/mdemg-mcp"
    echo ""
    echo "To stop: kill $MDEMG_PID"
else
    echo "ERROR: MDEMG service failed to start"
    exit 1
fi
