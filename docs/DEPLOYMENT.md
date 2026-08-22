# Production Deployment Guide — Docs Hub Next

This guide provides best practices and production instructions for deploying, securing, and maintaining **Docs Hub Next**.

---

## 1. Prerequisites & System Requirements

- **Operating System**: Linux (Ubuntu 22.04+, Debian 12+, RHEL 9+), macOS, or Windows Server.
- **Hardware**: Minimum 1 vCPU, 512 MB RAM, 10 GB SSD storage.
- **Runtime**: Docker & Docker Compose or a standalone precompiled binary.

---

## 2. Deployment Strategies

### Strategy A: Docker Compose with Automated Reverse Proxy (Recommended)

1. Create a dedicated directory:
   ```bash
   mkdir -p /opt/docshub && cd /opt/docshub
   ```

2. Configure environment file `.env`:
   ```env
   ADMIN_USER=admin
   ADMIN_PASSWORD=change_this_to_a_secure_password_min_8_chars
   SESSION_SECRET=generate_a_random_32_byte_hex_string_min_16_chars
   ADDR=:8080
   HOST_PORT=8080
   DATA_DIR=/data
   SITE_NAME="Acme Knowledge Hub"
   COOKIE_SECURE=1
   LOG_LEVEL=info
   RATE_LIMIT_ENABLED=true
   RATE_LIMIT_RPM=60
   RATE_LIMIT_BURST=15
   ```

3. Configure `compose.yaml`:
   ```yaml
   services:
     docshub:
       image: ghcr.io/homiakus/docshub:latest
       build: .
       container_name: docshub-app
       restart: unless-stopped
       env_file: .env
       ports:
         - "127.0.0.1:${HOST_PORT:-8080}:8080"
       volumes:
         - ./data:/data
       healthcheck:
         test: ["CMD", "wget", "-q", "-O", "-", "http://127.0.0.1:8080/healthz"]
         interval: 30s
         timeout: 5s
         retries: 3
         start_period: 10s
   ```

4. Start the stack:
   ```bash
   docker compose up -d
   ```

---

### Strategy B: Linux systemd Service (Bare-Metal / VM)

1. Build or download the binary:
   ```bash
   go build -ldflags="-s -w" -o /usr/local/bin/docshub ./cmd/docshub
   ```

2. Create a system user and data directory:
   ```bash
   useradd -r -s /bin/false docshub
   mkdir -p /var/lib/docshub /etc/docshub
   chown -R docshub:docshub /var/lib/docshub /etc/docshub
   ```

3. Create the configuration file `/etc/docshub/docshub.env`:
   ```env
   ADMIN_USER=admin
   ADMIN_PASSWORD=your_secure_password
   SESSION_SECRET=your_32_byte_secret
   ADDR=127.0.0.1:8080
   DATA_DIR=/var/lib/docshub
   COOKIE_SECURE=1
   ```

4. Create systemd unit `/etc/systemd/system/docshub.service`:
   ```ini
   [Unit]
   Description=Docs Hub Next Knowledge Engine
   After=network.target

   [Service]
   Type=simple
   User=docshub
   Group=docshub
   EnvironmentFile=/etc/docshub/docshub.env
   ExecStart=/usr/local/bin/docshub
   Restart=always
   RestartSec=5s
   LimitNOFILE=65536
   ProtectSystem=full
   ProtectHome=true
   NoNewPrivileges=true

   [Install]
   WantedBy=multi-user.target
   ```

5. Enable and start the service:
   ```bash
   systemctl daemon-reload
   systemctl enable --now docshub
   ```

---

## 3. Reverse Proxy & TLS Configuration

### Caddy (Automatic HTTPS)

```caddy
wiki.example.com {
    reverse_proxy 127.0.0.1:8080 {
        header_up X-Real-IP {remote_host}
        header_up X-Forwarded-Proto {scheme}
    }
}
```

### Nginx

```nginx
server {
    listen 80;
    server_name wiki.example.com;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    server_name wiki.example.com;

    ssl_certificate /etc/letsencrypt/live/wiki.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/wiki.example.com/privkey.pem;

    client_max_body_size 50M;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

---

## 4. Backups & Disaster Recovery

Docs Hub data is completely encapsulated within the `DATA_DIR` directory:
- Database: `docshub.db` (and temporary WAL files `docshub.db-wal`, `docshub.db-shm`)
- Media Uploads: `uploads/`

### Automated Daily Backup Script

```bash
#!/bin/bash
set -euo pipefail

BACKUP_DIR="/var/backups/docshub"
DATE=$(date +%Y%m%d_%H%M%S)
mkdir -p "$BACKUP_DIR"

# Perform SQLite safe online backup
sqlite3 /var/lib/docshub/docshub.db ".backup '${BACKUP_DIR}/docshub_${DATE}.db'"

# Archive uploaded assets
tar -czf "${BACKUP_DIR}/uploads_${DATE}.tar.gz" -C /var/lib/docshub uploads/

# Retain backups for 30 days
find "$BACKUP_DIR" -type f -mtime +30 -delete
```
