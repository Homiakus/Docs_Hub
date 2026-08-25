# Linux Production Manager

`manage-server.sh` is the interactive production deployment and maintenance utility for Docs Hub.

It deploys this topology:

```text
Internet
  |
  +-- TCP 80 / 443
        |
        v
      Caddy
        |
        v
  Docs Hub :8080
  (private Docker network)
```

Caddy is the TLS terminator. It obtains a publicly trusted ACME certificate automatically, redirects HTTP to HTTPS, and renews the certificate without a separate Certbot timer.

## Fast path on a new VPS

Clone the repository, then run:

```bash
git clone https://github.com/Homiakus/Docs_Hub.git
cd Docs_Hub
chmod +x manage-server.sh
./manage-server.sh
```

Choose **1 — Full production deploy**.

The manager will:

- inspect the server and detect port conflicts;
- install base packages on Debian/Ubuntu;
- install Docker Engine and the Compose plugin from Docker's official APT repository when required;
- prompt for the domain, ACME email, site name, administrator account and password;
- generate a 256-bit session secret;
- create `.env.production` with mode `0600`;
- check DNS A/AAAA resolution and compare the A record with the server public IPv4;
- optionally configure UFW;
- build Docs Hub and start Caddy;
- wait for the internal `/healthz` endpoint;
- wait for the public HTTPS endpoint and print certificate details.

## Required DNS and network state

Before certificate issuance:

1. Create an `A` record for the chosen hostname pointing to the VPS public IPv4.
2. If an `AAAA` record exists, make sure IPv6 really reaches the same server.
3. Allow inbound TCP 80 and 443 in the VPS firewall and cloud/provider firewall.
4. If the server is behind NAT, forward TCP 80 and 443 to it.

## Daily operations

The menu includes:

- production build/start;
- update from `origin/main` with a pre-update data backup;
- health and TLS diagnostics;
- live application/Caddy logs;
- restart/stop;
- consistent cold backup of SQLite/WAL data;
- restore with an automatic pre-restore safety backup;
- UFW configuration;
- system preflight.

Non-interactive shortcuts are also available:

```bash
./manage-server.sh deploy
./manage-server.sh status
./manage-server.sh update
./manage-server.sh backup
```

Set `DOCSHUB_BACKUP_DIR` to override `/var/backups/docs-hub`.

## Production files

- `compose.production.yaml` — production-only topology; the application port is not published to the host.
- `Caddyfile.production` — reverse proxy and automatic HTTPS configuration.
- `.env.production` — generated secret configuration; ignored by Git and mode `0600`.
- `data/` — persistent SQLite, WAL and uploads.
- Docker named volumes `docshub_caddy_data` and `docshub_caddy_config` — ACME state and certificates.

Do not delete the Caddy data volume unless you intentionally want to discard its ACME/certificate state.
