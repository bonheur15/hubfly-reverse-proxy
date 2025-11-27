# hubfly-reverse-proxy

A lightweight, single container reverse proxy appliance wrapping NGINX and Certbot with a Go based REST API. It provides safe, atomic configuration management and automated SSL certificate handling.

## How to Run

The easiest way to run Hubfly is using Docker Compose. This sets up the API, NGINX, and necessary volumes.

### Prerequisites
- Docker
- Docker Compose

### Start the Service
```bash
docker-compose up --build
```

- **API**: `http://localhost:81`
- **HTTP**: Port `80`
- **HTTPS**: Port `443`

---

## API Usage & Testing

Here are `curl` commands to interact with the API.

### 1. Check Health
Verify the service is running.
```bash
curl -i http://localhost:81/v1/health
```

### 2. Create a Simple Site (HTTP)
Forward traffic from `example.local` to a local upstream (e.g., a container IP or external site).
```bash
curl -X POST http://localhost:81/v1/sites \
  -H "Content-Type: application/json" \
  -d '{
    "id": "my-site-4",
    "domain": "example.local",
    "upstreams": ["127.0.0.1:9020"],
    "ssl": true,
    "force_ssl": true,
    "templates": ["security-headers"]
  }'
```
*Note: To test this locally, add `127.0.0.1 example.local` to your `/etc/hosts`.*

### 3. Create a Site with SSL (Production)
**Prerequisite:** The domain must point to this server's public IP, and port 80/443 must be open.
```bash "basic-caching", 
curl -X POST http://localhost:81/v1/sites \
  -H "Content-Type: application/json" \
  -d '{
    "id": "secure-site-1",
    "domain": "testing-33.hubfly.app",
    "upstreams": ["youthful_margulis:80"],
    "ssl": true,
    "force_ssl": true,
    "templates": ["security-headers","basic-caching"]
  }'
```

### 4. List All Sites
See all configured sites and their status.
```bash
curl http://localhost:81/v1/sites
```

### 5. Get Site Details
View configuration for a specific site.
```bash
curl http://localhost:81/v1/sites/my-site
```

### 6. Delete a Site
Remove the NGINX config. Add `?revoke_cert=true` to also revoke the SSL certificate.
```bash
curl -X DELETE http://localhost:81/v1/sites/secure-site-1?revoke_cert=true
# OR with revocation
# curl -X DELETE "http://localhost:81/v1/sites/secure-site?revoke_cert=true"
```

### 7. TCP/UDP Stream Proxying (Databases, SSH, etc.)
Hubfly can also proxy TCP and UDP traffic (Layer 4). This is useful for exposing databases, game servers, or other non-HTTP services.

**Important:** You must ensure the `listen_port` is exposed in your Docker container (e.g., via `-p` flags in `docker run` or `ports` in `docker-compose.yml`).

#### Basic TCP Stream (e.g., Postgres)
Forward traffic from an automatically assigned port (30000-30100) on the host to a container named `postgres_db` on port `5432`. If `listen_port` is omitted, it will be automatically assigned. The assigned port will be returned in the response.

```bash
curl -X POST http://localhost:81/v1/streams \
  -H "Content-Type: application/json" \
  -d '{
    "upstream": "jolly_kare:5432",
    "protocol": "tcp",
    "id":"jolly_kare:5432"
  }'
```

response:
{"id":"db-1:3306","listen_port":30073,"upstream":"db-1:3306","protocol":"tcp","status":"provisioning","created_at":"2025-11-27T12:40:20.176747778Z","updated_at":"2025-11-27T12:40:20.176747878Z"}


#### List Streams
```bash
curl http://localhost:81/v1/streams
```

#### Delete a Stream
```bash
# For a basic stream, the ID is typically 'stream-{port}' or manually provided
curl -X DELETE http://localhost:81/v1/streams/db-1:3306

# For an SNI stream, use the provided ID
curl -X DELETE http://localhost:81/v1/streams/mysql-db1
```

---

## Project Structure

- **/cmd/hubfly**: Main entry point.
- **/internal/api**: REST API handlers and routing.
- **/internal/nginx**: NGINX configuration generation, validation, and reloading.
- **/internal/certbot**: Wrapper for Certbot (SSL issuance/revocation).
- **/internal/store**: JSON-based persistence for site metadata.
- **/templates**: NGINX configuration snippets (e.g., caching, security).
