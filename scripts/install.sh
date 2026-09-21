#!/bin/sh
# Scout install for Raspberry Pi (ARM64): binary + systemd user service.
# The service runs the Scout MCP server; the human interface is the terminal:
# run `scout` locally or over SSH/Tailscale.
set -eu
cd "$(dirname "$0")/.."
go build -o scout ./cmd/scout
sudo install -m 0755 scout /usr/local/bin/scout
rm -f scout
mkdir -p ~/.local/share/scout
/usr/local/bin/scout init
systemctl --user enable --now scout 2>/dev/null || {
  sudo cp scripts/scout.service /etc/systemd/system/scout@.service
  echo "installed system unit scout@.service; run: sudo systemctl enable --now scout@$USER"
}
echo "Scout installed. Run 'scout' for the interactive session."
echo "MCP endpoint (Tailscale/loopback only): http://127.0.0.1:3210/mcp"
