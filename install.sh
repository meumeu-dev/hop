#!/bin/bash
# Install hop — https://github.com/meumeu-dev/hop
#
# Usage:
#   curl -sSL https://raw.githubusercontent.com/meumeu-dev/hop/master/install.sh | bash
set -euo pipefail

REPO="meumeu-dev/hop"

# Emplacement d'installation.
#
# Par defaut on privilegie ~/.local/bin : hop se veut sans privileges et sans
# trace (config en sandbox dans /tmp, `hop exit` qui efface tout), or exiger
# sudo pour poser le binaire contredit cette promesse — et `hop exit` devait
# lui aussi redemander sudo pour se supprimer.
#
# Ordre de preference :
#   1. $HOP_INSTALL_DIR si fourni (permet de tester sans toucher au systeme)
#   2. ~/.local/bin s'il est dans le PATH (ou s'il existe deja)
#   3. /usr/local/bin en dernier recours (sudo)
INSTALL_DIR=""

if [ -n "${HOP_INSTALL_DIR:-}" ]; then
    INSTALL_DIR="$HOP_INSTALL_DIR"
elif [ -n "${TERMUX_VERSION:-}" ] || [ -d "/data/data/com.termux" ]; then
    # Termux (Android) : pas de /usr/local/bin, pas de sudo
    INSTALL_DIR="$PREFIX/bin"
else
    USER_BIN="$HOME/.local/bin"
    case ":${PATH}:" in
        *":$USER_BIN:"*) INSTALL_DIR="$USER_BIN" ;;
        *)
            # Pas dans le PATH : on l'utilise quand meme s'il existe deja,
            # sinon on retombe sur l'emplacement systeme.
            if [ -d "$USER_BIN" ]; then
                INSTALL_DIR="$USER_BIN"
                PATH_HINT="$USER_BIN"
            else
                INSTALL_DIR="/usr/local/bin"
            fi
            ;;
    esac
fi

mkdir -p "$INSTALL_DIR" 2>/dev/null || true

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

# Verification d'integrite SHA256 — meme garantie que `hop update`, qui refuse
# deja d'installer un binaire dont la somme ne correspond pas. Sans cela, un
# `curl | bash` (souvent lance avec sudo) fait confiance a l'octet pres a tout
# ce qui sort du reseau.
echo "→ Verification de l'integrite..."
EXPECTED=$(curl -sSfL "https://github.com/$REPO/releases/download/$LATEST/$BINARY.sha256" | awk '{print $1}')
if [ -z "$EXPECTED" ]; then
    rm -f "$TMP"
    echo "Erreur: somme de controle introuvable pour $BINARY — installation annulee."
    exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
    ACTUAL=$(sha256sum "$TMP" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
    ACTUAL=$(shasum -a 256 "$TMP" | awk '{print $1}')
else
    rm -f "$TMP"
    echo "Erreur: ni sha256sum ni shasum disponible pour verifier le binaire — installation annulee."
    exit 1
fi

if [ "$EXPECTED" != "$ACTUAL" ]; then
    rm -f "$TMP"
    echo "Erreur: SOMME DE CONTROLE INVALIDE — binaire corrompu ou altere."
    echo "  Attendu: $EXPECTED"
    echo "  Obtenu:  $ACTUAL"
    exit 1
fi
echo "→ Integrite verifiee (SHA256)"

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

# Le dossier existait mais n'est pas dans le PATH : sans ca, `hop` reste
# introuvable et l'utilisateur croit l'installation ratee.
if [ -n "${PATH_HINT:-}" ]; then
    echo "⚠ $PATH_HINT n'est pas dans ton PATH. Ajoute cette ligne a ton shell :"
    echo "    export PATH=\"\$HOME/.local/bin:\$PATH\""
    echo ""
fi

echo "Si 'hop' ne fonctionne pas: hash -r ou ouvrez un nouveau terminal"
echo ""
echo "Demarrage: hop config"
