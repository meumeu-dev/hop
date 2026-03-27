#!/bin/bash
# Install hop — https://github.com/meumeu-dev/hop
#
# Usage:
#   curl -sSL https://raw.githubusercontent.com/meumeu-dev/hop/master/install.sh | bash
set -euo pipefail

REPO="meumeu-dev/hop"
INSTALL_DIR="/usr/local/bin"

# Detect arch
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)  BINARY="hop-linux-amd64" ;;
    aarch64) BINARY="hop-linux-arm64" ;;
    armv7l)  BINARY="hop-linux-arm32" ;;
    *)       echo "Architecture non supportee: $ARCH"; exit 1 ;;
esac

echo "→ Detection de la derniere version..."
LATEST=$(curl -sSf "https://api.github.com/repos/$REPO/releases/latest" | grep -o '"tag_name": *"[^"]*"' | cut -d'"' -f4)

if [ -z "$LATEST" ]; then
    echo "Erreur: impossible de trouver la derniere release"
    exit 1
fi

echo "→ Version: $LATEST"
echo "→ Telechargement $BINARY..."

TMP=$(mktemp)
curl -sSfL "https://github.com/$REPO/releases/download/$LATEST/$BINARY" -o "$TMP"
chmod +x "$TMP"

# Install
if [ -w "$INSTALL_DIR" ]; then
    mv "$TMP" "$INSTALL_DIR/hop"
else
    echo "→ Installation dans $INSTALL_DIR (sudo requis)..."
    sudo mv "$TMP" "$INSTALL_DIR/hop"
fi

echo "→ hop $LATEST installe dans $INSTALL_DIR/hop"
echo ""
echo "Demarrage: hop config"
