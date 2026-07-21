#!/usr/bin/env bash
#   ./build.sh            build both artifacts
#   ./build.sh deploy     build, then copy the exe to the Windows install dir
#
# Override the Windows install dir:
#   GATEWAY_DEPLOY_DIR=/mnt/c/Users/<you>/opencode-gateway ./build.sh deploy
set -euo pipefail
cd "$(dirname "$0")"

LDFLAGS="-H=windowsgui -s -w"
DEPLOY_DIR="${GATEWAY_DEPLOY_DIR:-/mnt/c/Users/ricardo/opencode-gateway}"

echo "==> Linux binary (plain / headless)"
go build -o opencode-gateway ./cmd/opencode-gateway

echo "==> Windows tray exe (cross-compiled)"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
	go build -tags tray -ldflags="$LDFLAGS" -o opencode-gateway.exe ./cmd/opencode-gateway

ls -lh opencode-gateway opencode-gateway.exe | awk '{printf "    %-24s %s\n", $9, $5}'

if [[ "${1:-}" == "deploy" ]]; then
	mkdir -p "$DEPLOY_DIR"
	if cp opencode-gateway.exe "$DEPLOY_DIR/opencode-gateway.exe" 2>/dev/null; then
		echo "==> deployed to $DEPLOY_DIR/opencode-gateway.exe"
	else
		cp opencode-gateway.exe "$DEPLOY_DIR/opencode-gateway-new.exe"
		echo "==> exe is locked (tray app running)."
		echo "    Staged as opencode-gateway-new.exe — quit the tray app, then replace"
		echo "    opencode-gateway.exe with it."
	fi
fi

echo "Done."
