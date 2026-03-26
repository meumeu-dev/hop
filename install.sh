#!/bin/bash
# Install hop from GitHub releases (private repo)
#
# Usage (gh CLI requis):
#   bash <(gh api repos/meumeu-dev/hop/contents/install.sh --jq '.content' | base64 -d)
#
# Ou si le script est deja sur la machine:
#   bash install.sh
#
# Ou avec token:
#   GITHUB_TOKEN=ghp_xxx bash install.sh
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

# Get GitHub token
TOKEN="${GITHUB_TOKEN:-${GH_TOKEN:-}}"
if [ -z "$TOKEN" ] && command -v gh &>/dev/null; then
    TOKEN=$(gh auth token 2>/dev/null || true)
fi

if [ -z "$TOKEN" ]; then
    echo "Erreur: token GitHub requis (repo prive)"
    echo ""
    echo "Options:"
    echo "  1. GITHUB_TOKEN=ghp_xxx bash install.sh"
    echo "  2. Installe gh CLI: gh auth login"
    echo "  3. export GITHUB_TOKEN=ghp_xxx"
    exit 1
fi

AUTH="Authorization: Bearer $TOKEN"

echo "→ Detection de la derniere version..."
LATEST=$(curl -sSf -H "$AUTH" -H "Accept: application/vnd.github.v3+json" \
    "https://api.github.com/repos/$REPO/releases/latest" | grep -o '"tag_name":"[^"]*"' | cut -d'"' -f4)

if [ -z "$LATEST" ]; then
    echo "Erreur: impossible de trouver la derniere release"
    exit 1
fi

echo "→ Version: $LATEST"
echo "→ Telechargement $BINARY..."

# Get asset download URL
ASSET_URL=$(curl -sSf -H "$AUTH" -H "Accept: application/vnd.github.v3+json" \
    "https://api.github.com/repos/$REPO/releases/tags/$LATEST" | \
    grep -B3 "\"name\":\"$BINARY\"" | grep "browser_download_url" | cut -d'"' -f4)

if [ -z "$ASSET_URL" ]; then
    echo "Erreur: binaire $BINARY non trouve dans la release $LATEST"
    exit 1
fi

TMP=$(mktemp)
curl -sSfL -H "$AUTH" -H "Accept: application/octet-stream" -o "$TMP" "$ASSET_URL"
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
