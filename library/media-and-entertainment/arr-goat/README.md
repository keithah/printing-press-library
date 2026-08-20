# arr-goat

Unified command-line control plane for your self-hosted Servarr ("arr") media
stack — **Sonarr, Radarr, Prowlarr, Bazarr, SABnzbd, and Transmission** — all
in one binary.

## Quick start

```bash
# 1. Install
go install github.com/mvanhorn/printing-press-library/library/media-and-entertainment/arr-goat/cmd/arr-goat@latest

# 2. Configure the fleet (~/.config/arr-goat/config.toml)
#    one [[services]] block per service: base_url + key_env (env var name)

# 3. Set key env vars
export SONARR_GOAT_X_API_KEY=…          # Servarr keys = X-Api-Key header
export RADARR_GOAT_X_API_KEY=…
export PROWLARR_GOAT_X_API_KEY=…
export BAZARR_GOAT_X_API_KEY=…
export SABNZBD_GOAT_X_API_KEY=…         # sabnzbd key = ?apikey= query param
export ARR_GOAT_TRANSMISSION_KEY="<user>:<pass>"   # transmission HTTP Basic user:pass

# 4. Use it
arr-goat status            # live probe all six services
arr-goat sonarr health     # one-service live status
arr-goat transmission health
arr-goat config            # show resolved fleet
arr-goat doctor            # verify keys + base urls
```

## Commands

| Command | Description |
|---------|-------------|
| `arr-goat status` | Live health/status probe of every configured service |
| `arr-goat <service> health` | One-line live status for a single service |
| `arr-goat config` | Show resolved fleet (base_url + key state) |
| `arr-goat doctor` | Verify config + credentials per service |
| `arr-goat transmission torrents [q]` / `add` / `start` / `stop` / `remove` | Torrent control |
| `arr-goat sabnzbd queue` / `pause` / `resume` | Download queue control |
| `arr-goat bazarr status` / `wanted [episodes\|movies]` | Subtitle status + missing |
| `arr-goat <service> <args...>` | Deep-dispatch to per-service engine (when installed) |

## Services

All reached over the homelab Cloudflare Tunnel (`https://hadm.net/<path>`).

| Service | Route | Auth | Live-verified |
|---------|-------|------|---------------|
| sonarr | /tv | X-Api-Key | ✅ 5,691 series |
| radarr | /movies | X-Api-Key | ✅ 17,751 movies |
| prowlarr | /prowlarr | X-Api-Key | ✅ indexer health |
| bazarr | /subtitles | X-Api-Key | ✅ v1.6.0 |
| sabnzbd | /sabnzbd | ?apikey= | ✅ v5.1.1, ~176 GB free |
| transmission | /transmission | HTTP Basic (RPC) | ✅ 683 torrents / 683 active |

## How it works

`arr-goat` is fully self-contained: `status` and `<service> health` use
in-process HTTP/RPC clients (no separate binaries). Each service's base URL
and credential come from one TOML config + env. Credentials are always by env
var name — never stored in the config file.

For deep per-service command surfaces, `arr-goat <service> <args...>` will
dispatch to an installed per-service engine (`sonarr-goat-pp-cli`, etc.) when
present; health/status always works regardless.
