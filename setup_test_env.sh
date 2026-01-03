#!/bin/bash
set -e

echo "--- Cleaning up previous runs ---"
docker compose down -v
docker rm -f simple-server 2>/dev/null || true
docker network rm simple-server-net 2>/dev/null || true

echo "--- Starting Hubfly Reverse Proxy ---"
docker compose up --build -d

echo "--- Creating network for Simple Server ---"
docker network create simple-server-net

echo "--- Starting Simple Server on its own network ---"
docker run -d \
  --name simple-server \
  --network simple-server-net \
  -p 4000:80 \
  bonheur15/hubfly-template-static-html:latest

echo "--- Connecting Hubfly Reverse Proxy to Simple Server Network ---"
# This mimics how Hubfly attaches to external project networks
docker network connect simple-server-net hubfly-reverse-proxy

echo "--- Setup Complete ---"
echo "Hubfly API: http://localhost:81"
echo "Simple Server: http://localhost:4000 (External)"
echo "Internal Hostname for Hubfly Upstream: 'simple-server'"