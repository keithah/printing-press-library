---
name: kuma-operations
description: Use when investigating or safely modifying Uptime Kuma v2 monitors.
---

# Uptime Kuma operations

Use `kuma-pp-cli` for authenticated Kuma v2 inspection. Credentials come from `UPTIME_KUMA_URL`, `UPTIME_KUMA_USERNAME`, and `UPTIME_KUMA_PASSWORD`.

## Investigation

1. Run `kuma-pp-cli health`.
2. Run `kuma-pp-cli monitors --json` and identify the exact monitor ID.
3. Run `kuma-pp-cli incident-context --monitor <id> --json`.
4. Use `kuma-pp-cli heartbeats --monitor-id <id> --hours 3 --json` for the timeline.

## Mutations

`set-retries` is dry-run by default. Review the proposed complete monitor update, then repeat the exact command with `--yes` only when explicitly authorized. Do not invent monitor IDs or alter notification settings as part of an unrelated retry change.
