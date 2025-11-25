# hubfly-reverse-proxy

A lightweight, single container reverse proxy appliance wrapping NGINX and Certbot with a Go based REST API. It provides safe, atomic configuration management and automated SSL certificate handling.

## 🚀 How to Run

The easiest way to run Hubfly is using Docker Compose. This sets up the API, NGINX, and necessary volumes.

### Prerequisites
- Docker
- Docker Compose

### Start the Service
```bash
docker-compose up --build
```

- **API**: `http://localhost:6000`
- **HTTP**: Port `80`
- **HTTPS**: Port `443`

---

## 🧪 API Usage & Testing

Here are `curl` commands to interact with the API.

### 1. Check Health
Verify the service is running.
```bash
curl -i http://localhost:6000/v1/health
```

### 2. Create a Simple Site (HTTP)
Forward traffic from `example.local` to a local upstream (e.g., a container IP or external site).
```bash
curl -X POST http://localhost:6000/v1/sites \
  -H "Content-Type: application/json" \
  -d '{
    "id": "my-site-4",
    "domain": "testing-3333.hubfly.app",
    "upstreams": ["127.0.0.1:9020"],
    "ssl": true,
    "force_ssl": true,
    "templates": ["security-headers"]
  }'
```
*Note: To test this locally, add `127.0.0.1 example.local` to your `/etc/hosts`.*

### 3. Create a Site with SSL (Production)
**Prerequisite:** The domain must point to this server's public IP, and port 80/443 must be open.
```bash
curl -X POST http://localhost:6000/v1/sites \
  -H "Content-Type: application/json" \
  -d '{
    "id": "secure-site",
    "domain": "mysite.com",
    "upstreams": ["10.0.0.5:3000"],
    "ssl": true,
    "force_ssl": true,
    "templates": ["basic-caching", "security-headers"]
  }'
```

### 4. List All Sites
See all configured sites and their status.
```bash
curl http://localhost:6000/v1/sites
```

### 5. Get Site Details
View configuration for a specific site.
```bash
curl http://localhost:6000/v1/sites/my-site
```

### 6. Delete a Site
Remove the NGINX config. Add `?revoke_cert=true` to also revoke the SSL certificate.
```bash
curl -X DELETE http://localhost:6000/v1/sites/my-site
# OR with revocation
# curl -X DELETE "http://localhost:6000/v1/sites/secure-site?revoke_cert=true"
```

---

## 📂 Project Structure

- **/cmd/hubfly**: Main entry point.
- **/internal/api**: REST API handlers and routing.
- **/internal/nginx**: NGINX configuration generation, validation, and reloading.
- **/internal/certbot**: Wrapper for Certbot (SSL issuance/revocation).
- **/internal/store**: JSON-based persistence for site metadata.
- **/templates**: NGINX configuration snippets (e.g., caching, security).
