#!/bin/bash

# Wrapper script for OpenAdServe MCP Server using official Go SDK
# This runs the MCP server on-demand in the dedicated container

# Change to the directory containing this script (project root)
cd "$(dirname "$0")"

# Check if containers are running
if ! docker compose ps mcp-server | grep -q "Up"; then
    echo "Error: MCP server container is not running. Please run 'docker compose up -d' first." >&2
    exit 1
fi

# Execute the MCP server on-demand in the mcp-server container
exec docker compose exec -T mcp-server ./mcp-server