#!/bin/bash
# Install hop — https://github.com/meumeu-dev/hop
#
# Usage:
#   curl -sSL https://raw.githubusercontent.com/meumeu-dev/hop/master/install.sh | bash
set -euo pipefail

REPO="meumeu-dev/hop"
INSTALL_DIR="/usr/local/bin"

# Detect Termux (Android)
if [ -n "$TERMUX_VERSION" ] || [ -d "/data/data/com.termux" ]; then
    INSTALL_DIR="$PREFIX/bin"
fi

# Detect OS
OS=$(uname -s)
case "$OS" in
    Linux)  OS_NAME="linux" ;;
    Darwin) OS_NAME="darwin" ;;
    *)      echo "OS non supporte: $OS"; exit 1 ;;
esac

# Detect arch
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)  ARCH_NAME="amd64" ;;
    aarch64) ARCH_NAME="arm64" ;;
    arm64)   ARCH_NAME="arm64" ;;
    armv7l)  ARCH_NAME="arm32" ;;
    *)       echo "Architecture non supportee: $ARCH"; exit 1 ;;
esac

# arm32 only exists for linux
if [ "$OS_NAME" = "darwin" ] && [ "$ARCH_NAME" = "arm32" ]; then
    echo "Architecture non supportee sur macOS: $ARCH"
    exit 1
fi

BINARY="hop-${OS_NAME}-${ARCH_NAME}"

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
echo "Si 'hop' ne fonctionne pas: hash -r ou ouvrez un nouveau terminal"
echo ""
echo "Demarrage: hop config"
