#!/bin/bash
# Start MDEMG Memory System for IDE Agent Integration
#
# Prerequisites:
#   - Docker running with Neo4j container
#   - Ollama running with nomic-embed-text model
#
# Usage: ./start-mdemg.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVICE_DIR="$SCRIPT_DIR/mdemg_build/service"

echo "=== MDEMG Memory System Startup ==="

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

# Check Ollama
echo -n "Checking Ollama... "
if curl -s http://localhost:11434/api/tags >/dev/null 2>&1; then
    echo "OK (running)"
else
    echo "WARNING: Ollama not running. Start with: ollama serve"
fi

# Check embedding model
echo -n "Checking embedding model... "
if curl -s http://localhost:11434/api/tags 2>/dev/null | grep -q "nomic-embed-text"; then
    echo "OK (nomic-embed-text)"
else
    echo "Pulling nomic-embed-text model..."
    ollama pull nomic-embed-text
fi

# Build and start MDEMG service
echo "Building MDEMG service..."
cd "$SERVICE_DIR"
go build -o /tmp/mdemg-server ./cmd/server

echo "Starting MDEMG service on :8082..."
export NEO4J_URI=bolt://localhost:7687
export NEO4J_USER=neo4j
export NEO4J_PASS=testpassword
export REQUIRED_SCHEMA_VERSION=4
export LISTEN_ADDR=:8082
export EMBEDDING_PROVIDER=ollama
export OLLAMA_MODEL=nomic-embed-text

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
