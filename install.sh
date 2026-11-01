#!/usr/bin/env bash
set -e

# WaterStick Installer for Linux and macOS
REPO="MedCy1/WaterStick"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

# Detect OS
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  linux*)  GOOS="linux" ;;
  darwin*) GOOS="darwin" ;;
  *) echo "Unsupported operating system: $OS" && exit 1 ;;
esac

# Detect Architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  GOARCH="amd64" ;;
  aarch64|arm64) GOARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH" && exit 1 ;;
esac

echo "Determining latest WaterStick release..."
AUTH_HEADER=()
if [ -n "$GITHUB_TOKEN" ]; then
  AUTH_HEADER=(-H "Authorization: Bearer $GITHUB_TOKEN")
elif [ -n "$GH_TOKEN" ]; then
  AUTH_HEADER=(-H "Authorization: Bearer $GH_TOKEN")
fi

TAG="$(curl -sSL "${AUTH_HEADER[@]}" "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')"

if [ -z "$TAG" ]; then
  echo "Failed to fetch latest release tag from GitHub."
  exit 1
fi

ARCHIVE_NAME="waterstick_${TAG}_${GOOS}_${GOARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${TAG}/${ARCHIVE_NAME}"

echo "Installing WaterStick ${TAG} (${GOOS}/${GOARCH})..."
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

curl -sSL "${AUTH_HEADER[@]}" "$DOWNLOAD_URL" -o "$TMP_DIR/$ARCHIVE_NAME"
tar -xzf "$TMP_DIR/$ARCHIVE_NAME" -C "$TMP_DIR"

mkdir -p "$INSTALL_DIR"
install -m 755 "$TMP_DIR/waterstick" "$INSTALL_DIR/waterstick"
install -m 755 "$TMP_DIR/waterstick-tui" "$INSTALL_DIR/waterstick-tui"

echo "WaterStick successfully installed into $INSTALL_DIR:"
echo "  - $INSTALL_DIR/waterstick (CLI)"
echo "  - $INSTALL_DIR/waterstick-tui (Dashboard)"

# Automatically install shell autocompletion
"$INSTALL_DIR/waterstick" completion install >/dev/null 2>&1 || true

# On Linux, create desktop entry for application launchers (Rofi, Wofi, HyDE, GNOME, KDE)
if [ "$GOOS" = "linux" ]; then
  DESKTOP_DIR="$HOME/.local/share/applications"
  if mkdir -p "$DESKTOP_DIR" 2>/dev/null; then
    cat <<EOF > "$DESKTOP_DIR/waterstick.desktop"
[Desktop Entry]
Name=WaterStick
Comment=Fire TV Stick 4K Max Debloat & Optimization Suite
Exec=$INSTALL_DIR/waterstick-tui
Terminal=true
Type=Application
Icon=utilities-terminal
Categories=Utility;System;
Keywords=firetv;adb;debloat;optimization;
EOF
    chmod 644 "$DESKTOP_DIR/waterstick.desktop" 2>/dev/null || true
    if command -v update-desktop-database >/dev/null 2>&1; then
      update-desktop-database "$DESKTOP_DIR" >/dev/null 2>&1 || true
    fi
  fi
fi

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    echo ""
    echo "Note: $INSTALL_DIR is not in your PATH."
    echo "Add it to your shell configuration (e.g. ~/.bashrc or ~/.zshrc):"
    echo "  export PATH=\"\$PATH:$INSTALL_DIR\""
    ;;
esac
