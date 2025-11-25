# Stage 1: Builder
FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod ./
# COPY go.sum ./ # No go.sum yet as we haven't run tidy/get with network, but good practice
RUN go mod download || true 
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/hubfly ./cmd/hubfly

# Stage 2: Runtime
FROM nginx:stable-alpine
# Install Certbot and dependencies
RUN apk add --no-cache certbot openssl bash ca-certificates

# Copy binary
COPY --from=builder /out/hubfly /usr/local/bin/hubfly

# Copy default nginx config (wrapper)
COPY ./nginx/nginx.conf /etc/nginx/nginx.conf

# Create necessary directories
RUN mkdir -p /etc/hubfly/sites /etc/hubfly/staging /etc/hubfly/templates \
    /var/www/hubfly /var/log/hubfly /var/cache/nginx

# Expose ports
EXPOSE 80 443 8080

# Volume for persistence
VOLUME ["/etc/letsencrypt", "/etc/hubfly", "/var/www/hubfly"]

# Entrypoint
CMD ["/usr/local/bin/hubfly", "--config-dir", "/etc/hubfly"]
