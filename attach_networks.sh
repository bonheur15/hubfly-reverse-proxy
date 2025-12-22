#!/bin/bash

CONTAINER_NAME="hubfly-reverse-proxy"
PREFIX="proj-network-proj_"

echo "--- Re-attaching Project Networks ---"
echo "Target Container: $CONTAINER_NAME"
echo "Network Prefix:   $PREFIX"

# Check if container is running
if ! docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    echo "Error: Container $CONTAINER_NAME is not running."
    exit 1
fi

# Find matching networks
NETWORKS=$(docker network ls --format '{{.Name}}' | grep "^${PREFIX}")

if [ -z "$NETWORKS" ]; then
    echo "No networks found matching prefix '$PREFIX'."
    exit 0
fi

COUNT=0
for NET in $NETWORKS; do
    echo "Attaching network: $NET"
    # Try to connect, ignore error if already connected (exit code 1 if connected)
    if docker network connect "$NET" "$CONTAINER_NAME" 2>/dev/null; then
        echo "  -> Successfully attached."
    else
        # check if it was because it's already connected
        if docker network inspect "$NET" | grep -q "$CONTAINER_NAME"; then
             echo "  -> Already connected."
        else
             echo "  -> Failed to connect."
        fi
    fi
    COUNT=$((COUNT+1))
done

echo "Processed $COUNT networks."
echo "-------------------------------------"
