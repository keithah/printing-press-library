---
name: pp-homeclaw
description: "Printing Press wrapper for HomeClaw macOS HomeKit CLI and stdio MCP server"
author: "Keith Herrington"
license: "MIT"
argument-hint: "<homeclaw-cli args> | mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - homeclaw-pp-cli
---

# HomeClaw — Printing Press wrapper

Use `homeclaw-pp-cli` to delegate to the HomeClaw app's existing CLI. HomeClaw must already be installed and running; this wrapper never installs the app, creates symlinks, asks for credentials, or grants HomeKit access.

```bash
homeclaw-pp-cli --help
homeclaw-pp-cli mcp
```

For a nonstandard installation, set `HOMECLAW_CLI_PATH`, `HOMECLAW_MCP_SERVER_PATH`, or `HOMECLAW_NODE_PATH`. For Hermes setup, use the portable plugin and `mcp.json` at the HomeClaw repository root.
