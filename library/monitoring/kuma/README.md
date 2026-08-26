# kuma-pp-cli

`kuma-pp-cli` is a small operator CLI for Uptime Kuma v2. It uses Kuma's Socket.IO protocol rather than pretending the dashboard is a REST API.

## Configuration

Set `UPTIME_KUMA_URL`, `UPTIME_KUMA_USERNAME`, and `UPTIME_KUMA_PASSWORD` (or pass `--url`, `--username`, and `--password`). Never commit credentials.

## Commands

```text
kuma-pp-cli health
kuma-pp-cli monitors [--query text] [--json]
kuma-pp-cli heartbeats [--hours 3] [--monitor-id N] [--json]
kuma-pp-cli incident-context --monitor id-or-name [--lookback-minutes 60] [--json]
kuma-pp-cli set-retries --monitor id-or-name --maxretries N       # dry-run
kuma-pp-cli set-retries --monitor id-or-name --maxretries N --yes
kuma-pp-cli version
kuma-pp-cli --version
```

All mutations are operator-gated and dry-run by default. The client sends complete monitor objects for `editMonitor`, because Kuma v2 treats edits as full replacement.

## Development

```bash
go test ./...
go vet ./...
go build ./...
```
