#!/bin/bash

# Minify JavaScript SDK using Node.js terser
# This script is called by pre-commit hook when adsdk.js changes

set -e

SDK_SOURCE="static/sdk/adsdk.js"
SDK_MINIFIED="static/sdk/adsdk.min.js"

# Check if Node.js is available
if ! command -v node &> /dev/null; then
    echo "Error: Node.js is required for JavaScript minification"
    echo "Please install Node.js: https://nodejs.org/"
    exit 1
fi

# Check if npx is available
if ! command -v npx &> /dev/null; then
    echo "Error: npx is required (should come with Node.js)"
    exit 1
fi

echo "Minifying JavaScript SDK..."
echo "Source: $SDK_SOURCE"
echo "Output: $SDK_MINIFIED"

# Install terser if not available and minify
npx terser@latest "$SDK_SOURCE" \
    --compress \
    --mangle \
    --source-map "filename='adsdk.min.js.map',url='adsdk.min.js.map'" \
    --output "$SDK_MINIFIED"

# Show file size comparison
if [[ -f "$SDK_SOURCE" && -f "$SDK_MINIFIED" ]]; then
    original_size=$(wc -c < "$SDK_SOURCE")
    minified_size=$(wc -c < "$SDK_MINIFIED")
    reduction=$((100 - (minified_size * 100 / original_size)))
    
    echo "✅ Minification complete!"
    echo "Original size: ${original_size} bytes"
    echo "Minified size: ${minified_size} bytes" 
    echo "Size reduction: ${reduction}%"
    
    # Add minified file to git staging
    git add "$SDK_MINIFIED" static/sdk/adsdk.min.js.map
else
    echo "❌ Minification failed"
    exit 1
fi