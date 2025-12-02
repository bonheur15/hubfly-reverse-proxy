#!/bin/bash
set -e

# Configuration
IMAGE_NAME="hubfly-reverse-proxy"
TAG="latest"
DEPLOY_DIR="~/hubfly-reverse-proxy"

# Check arguments
SERVER=$1
if [ -z "$SERVER" ]; then
  echo "Usage: ./deploy.sh <user@host>"
  echo "Example: ./deploy.sh root@192.168.1.10"
  exit 1
fi

echo "--- Starting Deployment to $SERVER ---"

# 1. Build the Docker image locally
echo "[1/5] Building Docker image locally..."
docker build -t ${IMAGE_NAME}:${TAG} .

# 2. Save and Compress Image
echo "[2/5] Saving and compressing image locally..."
docker save ${IMAGE_NAME}:${TAG} | gzip > ${IMAGE_NAME}.tar.gz

# 3. Transfer the image
echo "[3/5] Transferring image to $SERVER..."
scp ${IMAGE_NAME}.tar.gz "$SERVER:$DEPLOY_DIR/${IMAGE_NAME}.tar.gz"

# 4. Generate a production docker-compose file
echo "[4/5] Generating production compose file..."
cat > docker-compose.prod.yml <<EOF
version: "3.8"

services:
  hubfly:
    image: ${IMAGE_NAME}:${TAG}
    container_name: hubfly-reverse-proxy
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
      - "81:81"
      - "30000-30100:30000-30100"
    volumes:
      - hubfly_proxy_data:/etc/hubfly
      - hubfly_proxy_certs:/etc/letsencrypt
      - hubfly_proxy_webroot:/var/www/hubfly
    environment:
      - TZ=UTC
    networks:
      - hubfly-proxy-network

volumes:
  hubfly_proxy_data:
    name: hubfly_proxy_data
  hubfly_proxy_certs:
    name: hubfly_proxy_certs
  hubfly_proxy_webroot:
    name: hubfly_proxy_webroot

networks:
  hubfly-proxy-network:
    name: hubfly-proxy-network
    driver: bridge
EOF

# 5. Deploy on remote
echo "[5/5] Deploying on server..."
ssh "$SERVER" "mkdir -p $DEPLOY_DIR"
scp docker-compose.prod.yml "$SERVER:$DEPLOY_DIR/docker-compose.yml"
rm docker-compose.prod.yml

ssh "$SERVER" "cd $DEPLOY_DIR && \
  gunzip -c ${IMAGE_NAME}.tar.gz | docker load && \
  rm ${IMAGE_NAME}.tar.gz && \
  docker compose up -d || docker-compose up -d"

# Cleanup local
rm ${IMAGE_NAME}.tar.gz

echo "--- Deployment Complete! ---"
