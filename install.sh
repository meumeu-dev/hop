#!/bin/bash
# Install hop from GitHub releases (private repo)
#
# Usage (gh CLI requis):
#   bash <(gh api repos/meumeu-dev/hop/contents/install.sh --jq '.content' | base64 -d)
#
# Ou si le script est deja sur la machine:
#   bash install.sh
set -euo pipefail

REPO="meumeu-dev/hop"
INSTALL_DIR="/usr/local/bin"

# Require gh CLI
if ! command -v gh &>/dev/null; then
    echo "Erreur: gh CLI requis (https://cli.github.com/)"
    echo "  sudo apt install gh && gh auth login"
    exit 1
fi

# Detect arch
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)  BINARY="hop-linux-amd64" ;;
    aarch64) BINARY="hop-linux-arm64" ;;
    armv7l)  BINARY="hop-linux-arm32" ;;
    *)       echo "Architecture non supportee: $ARCH"; exit 1 ;;
esac

echo "→ Detection de la derniere version..."
LATEST=$(gh api "repos/$REPO/releases/latest" --jq '.tag_name')

if [ -z "$LATEST" ]; then
    echo "Erreur: impossible de trouver la derniere release"
    exit 1
fi

echo "→ Version: $LATEST"
echo "→ Telechargement $BINARY..."

TMP=$(mktemp)
gh release download "$LATEST" --repo "$REPO" --pattern "$BINARY" --output "$TMP" --clobber
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
echo "Demarrage: hop init"
