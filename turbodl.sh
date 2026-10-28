#!/usr/bin/env bash
# ==============================================================================
# TURBODL // 1-CLICK LINUX LAUNCHER
# Run via: ./turbodl.sh
# ==============================================================================

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN="$DIR/turbodl"

if [ ! -f "$BIN" ]; then
    echo "[*] Compiling TurboDL native binary..."
    if command -v go >/dev/null 2>&1; then
        (cd "$DIR" && go build -o turbodl ./cmd/turbodl)
    else
        echo "Go compiler not found. Downloading pre-compiled binary..."
        curl -fsSL -o "$BIN" "https://github.com/Lethal-Aggression/An-extremly-fast-Download-manager/releases/latest/download/turbodl"
        chmod +x "$BIN"
    fi
fi

exec "$BIN" "$@"
