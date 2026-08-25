# Local deployment — Docs Hub

Docs Hub has a local Docker deployment mode that is isolated from production.

## Quick start

```bash
git clone https://github.com/Homiakus/Docs_Hub.git
cd Docs_Hub
chmod +x deploy.sh manage-local.sh manage-server.sh
./deploy.sh
```

Choose **Local machine** and then **Full local deployment**.

Direct command:

```bash
./deploy.sh local deploy
```

## Local topology

```text
Browser -> 127.0.0.1:8080 -> Docker -> Docs Hub :8080
```

The default binding is `127.0.0.1`, so other machines cannot connect. The menu can switch to `0.0.0.0` for access from a trusted LAN.

Local mode intentionally uses plain HTTP and sets `COOKIE_SECURE=0`. Production keeps a separate Caddy/ACME HTTPS configuration.

## Isolation from production

| Mode | Env file | Compose file | Data |
| --- | --- | --- | --- |
| Local | `.env.local` | `compose.local.yaml` | `./data-local` |
| Production | `.env.production` | `compose.production.yaml` | `./data` |

This prevents local testing from modifying the production SQLite database or uploads.

## Local menu

The local manager supports:

- initial configuration;
- localhost-only or LAN binding;
- host port selection;
- secure admin password/session secret generation;
- Docker/Compose diagnostics;
- build and start;
- healthcheck and status;
- browser launch;
- live logs;
- restart/stop;
- explicit reset of local-only data.

## CLI

```bash
./manage-local.sh deploy
./manage-local.sh config
./manage-local.sh status
./manage-local.sh logs
./manage-local.sh restart
./manage-local.sh stop
```

## Windows

The repository also contains `manage.ps1` for native PowerShell/Docker Desktop workflows:

```powershell
.\manage.ps1
```

The Bash local manager is intended for Linux, macOS, WSL, and other environments with Bash + Docker Compose.
