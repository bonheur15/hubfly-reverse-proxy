#!/bin/bash
set -e

# Start NGINX
# We rely on standard /etc/nginx/nginx.conf. 
# Ensure 'daemon on;' is used or implied so it backgrounds itself, 
# OR we run it in background if 'daemon off;' is set.
# The nginx Docker image defaults to "daemon off;" usually in its CMD, 
# but since we replaced CMD, we control it.
# Our custom nginx.conf does not specify 'daemon', so it defaults to 'on' (daemonize).
echo "Starting NGINX..."
nginx

# Start Hubfly
echo "Starting Hubfly..."
exec /usr/local/bin/hubfly --config-dir /etc/hubfly
