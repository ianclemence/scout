#!/bin/sh
# Scout install for Raspberry Pi (ARM64): user-level binary + systemd service.
# No root needed. The service runs the Scout MCP server; the human interface
# is the terminal: run `scout` locally or over SSH/Tailscale.
set -eu
cd "$(dirname "$0")/.."
make build
install -m 0755 build/scout ~/.local/bin/scout
mkdir -p ~/.local/share/scout ~/.config/systemd/user
~/.local/bin/scout init
cp scripts/scout.service ~/.config/systemd/user/scout.service
systemctl --user daemon-reload
systemctl --user enable --now scout
echo "Scout installed. Run 'scout' for the interactive session."
echo "MCP endpoint (loopback only): http://127.0.0.1:3210/mcp"
