# Native deployment — without Docker

Docs Hub can run directly as a Go binary. Docker, Docker Compose, Caddy, Node.js, Python, and an external database are not required at runtime.

## Requirements

- Go **1.25.0+** (the version is read from `go.mod` by the manager);
- network access on the first build so Go can download modules;
- Bash for `manage-native.sh`, or Windows PowerShell 5.1+ for `manage-native.ps1`.

SQLite is embedded through the pure-Go `modernc.org/sqlite` driver, so a separate SQLite server/library installation is not required.

## Quick start — Linux/macOS/WSL

```bash
git clone https://github.com/Homiakus/Docs_Hub.git
cd Docs_Hub
chmod +x deploy.sh manage-native.sh
./deploy.sh
```

Choose:

```text
2) Native — WITHOUT Docker (Go binary, localhost/LAN)
```

Direct deployment:

```bash
./deploy.sh native deploy
```

## Quick start — Windows PowerShell

```powershell
git clone https://github.com/Homiakus/Docs_Hub.git
cd Docs_Hub
.\deploy.ps1
```

Choose:

```text
2) Native — WITHOUT Docker (Go binary)
```

Or directly:

```powershell
.\manage-native.ps1 -Action Deploy
```

## Native topology

```text
Browser
  |
  | HTTP :8080 (configurable)
  v
Docs Hub native Go process
  |
  v
data-native/ (SQLite + uploads)
```

The default bind address is `127.0.0.1`, so the application is reachable only from the current computer. The manager can switch to `0.0.0.0` for a trusted LAN.

Native local mode intentionally uses plain HTTP and `COOKIE_SECURE=0`. Do not expose this mode directly to the public Internet. For public production use the production mode with Caddy/automatic HTTPS, or put the native process behind a hardened reverse proxy/service manager.

## Isolation

| Mode | Config | Runtime | Data |
| --- | --- | --- | --- |
| Native / no Docker | `.env.native` | `bin/docshub-native[.exe]` | `./data-native` |
| Local Docker | `.env.local` | Docker Compose | `./data-local` |
| Production | `.env.production` | Docker + Caddy | `./data` |

Runtime state is also isolated:

```text
.run/docshub-native.pid
.logs/docshub-native*.log
```

All native runtime files and data directories are git-ignored.

## Native manager capabilities

The manager provides:

- Go version preflight against `go.mod`;
- secure `SESSION_SECRET` generation;
- generated or user-defined admin password;
- localhost-only or trusted-LAN binding;
- configurable port;
- `go mod download` + optimized `go build`;
- native process start/status/restart/stop;
- PID tracking;
- application `/healthz` check;
- log following;
- browser launch;
- `go test ./...`;
- explicit reset of **native-only** data.

## Bash CLI

```bash
./manage-native.sh deploy
./manage-native.sh config
./manage-native.sh build
./manage-native.sh start
./manage-native.sh status
./manage-native.sh logs
./manage-native.sh restart
./manage-native.sh stop
./manage-native.sh test
./manage-native.sh doctor
```

## PowerShell CLI

```powershell
.\manage-native.ps1 -Action Deploy
.\manage-native.ps1 -Action Configure
.\manage-native.ps1 -Action Build
.\manage-native.ps1 -Action Start
.\manage-native.ps1 -Action Status
.\manage-native.ps1 -Action Logs
.\manage-native.ps1 -Action Restart
.\manage-native.ps1 -Action Stop
.\manage-native.ps1 -Action Test
.\manage-native.ps1 -Action Doctor
```

## Build output

Linux/macOS/WSL:

```text
bin/docshub-native
```

Windows:

```text
bin\docshub-native.exe
```

The build uses:

```bash
go build -trimpath -ldflags="-s -w" ./cmd/docshub
```

This strips debug/symbol metadata and keeps the standalone executable compact.
