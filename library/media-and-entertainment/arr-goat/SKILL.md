---
name: pp-arr-goat
description: "Use when checking or driving the user's self-hosted Servarr media stack (Sonarr, Radarr, Prowlarr, Bazarr, SABnzbd, Transmission) from one command. `arr-goat status` probes every service live; per-service health + deep commands included. Trigger phrases: `arr-goat`, `check my sonarr`, `what's downloading`, `sonarr health`, `radarr library`, `my arr stack`, `transmission status`."
author: "Keith Herrington"
license: "Apache-2.0"
metadata:
  openclaw:
    requires:
      bins:
        - arr-goat
    install:
      - kind: go
        bins: [arr-goat]
---

# arr-goat

## What

`arr-goat` is a single-command control plane for the whole self-hosted Servarr
("arr") media stack: **Sonarr, Radarr, Prowlarr, Bazarr, SABnzbd, and
Transmission**. Everything is built into one binary — there are no separate
per-service CLIs. It reads one fleet config (`name → base_url + credential`),
then health/status and per-service commands all work from `arr-goat`.

Services are self-hosted and reached over the user's homelab Cloudflare Tunnel
(`https://hadm.net/tv` Sonarr, `/movies` Radarr, `/prowlarr`, `/subtitles`
Bazarr, `/sabnzbd`, `/transmission`).

## Configuration

Config file (default `~/.config/arr-goat/config.toml`, override `ARR_GOAT_CONFIG`):

```toml
[[services]]
name = "sonarr"
base_url = "https://hadm.net/tv"
key_env = "SONARR_GOAT_X_API_KEY"

[[services]]
name = "radarr"
base_url = "https://hadm.net/movies"
key_env = "RADARR_GOAT_X_API_KEY"

[[services]]
name = "prowlarr"
base_url = "https://hadm.net/prowlarr"
key_env = "PROWLARR_GOAT_X_API_KEY"

[[services]]
name = "bazarr"
base_url = "https://hadm.net/subtitles"
key_env = "BAZARR_GOAT_X_API_KEY"

[[services]]
name = "sabnzbd"
base_url = "https://hadm.net/sabnzbd"
key_env = "SABNZBD_GOAT_X_API_KEY"

[[services]]
name = "transmission"
base_url = "https://hadm.net/transmission"
key_env = "ARR_GOAT_TRANSMISSION_KEY"
```

Credentials are referenced by env var name (`key_env`), never inline. Default
key env per service is `ARR_GOAT_<NAME>_KEY`. Transmission uses HTTP Basic, so
its key is the `user:pass` pair (e.g. `<user>:<pass>`).

## Commands

- `arr-goat status` — live probe every service in one shot
- `arr-goat config` — show resolved fleet (base_url + key state per service)
- `arr-goat doctor` — verify key + base-url configured per service
- `arr-goat <service> health` — live one-line status for a single service

### Deep command surfaces (built into the binary)

**Transmission**
- `arr-goat transmission torrents [search] [--json]` — list torrents (filter by name)
- `arr-goat transmission add <magnet-or-url>` — add a torrent
- `arr-goat transmission start <ids...|all>` / `stop <ids...>`
- `arr-goat transmission remove <ids...> [--delete]` / `remove all --yes`
  (removing ALL torrents requires `--yes`; `--delete` also deletes local data)

**SABnzbd**
- `arr-goat sabnzbd queue [--json]` — show the download queue
- `arr-goat sabnzbd pause` / `resume` — pause/resume the queue

**Bazarr**
- `arr-goat bazarr status [--json]` — Bazarr + Sonarr/Radarr versions
- `arr-goat bazarr wanted [episodes|movies] [--json]` — list missing-subtitle items

`arr-goat status [--json]` prints all services; `--json` emits machine-readable
JSON across status/torrents/queue/wanted. Mutating commands (add/start/stop/
remove/pause) are not run unless explicitly asked.

Launched while the per-service engine binaries are present, `arr-goat
<service> <args...>` also deep-dispatches (sonarr/radarr/prowlarr have full
printed engines); with the engine absent it still works for `health`.

## Notes

- Live-verified against the real fleet: Sonarr (5,691 series), Radarr
  (17,751 movies), Prowlarr (indexer health), Bazarr (v1.6.0), SABnzbd
  (queue, ~176 GB free), Transmission (683 torrents / 683 active).
- Transmission is reached via its RPC (CSRF session-id handshake + HTTP Basic).
  Its host password was reset and Sonarr/Radarr download clients were updated
  to match.
- Bazarr/SABnzbd/Transmission deep surfaces (torrents/queue/wanted) are built
  into the binary; sonarr/radarr/prowlarr also deep-dispatch to their printed
  engines when installed.
- Do not invoke mutating commands (add/start/stop/remove/pause) unless
  explicitly asked.
