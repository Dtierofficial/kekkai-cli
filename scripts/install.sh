#!/usr/bin/env sh
set -eu

REPO='Dtierofficial/kekkai-cli'
INSTALL_DIR="$HOME/.kekkai/bin"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$OS" in
  linux) RELEASE_OS='linux' ;;
  darwin) RELEASE_OS='darwin' ;;
  *) printf '%s\n' "Unsupported operating system: $OS" >&2; exit 1 ;;
esac
case "$ARCH" in
  x86_64|amd64) RELEASE_ARCH='amd64' ;;
  aarch64|arm64) RELEASE_ARCH='arm64' ;;
  *) printf '%s\n' "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

command -v curl >/dev/null 2>&1 || { printf '%s\n' 'curl is required.' >&2; exit 1; }
API="https://api.github.com/repos/$REPO/releases/latest"
ASSET_URL=$(curl -fsSL -H 'Accept: application/vnd.github+json' -H 'User-Agent: kekkai-installer' "$API" \
  | sed -nE 's/.*"browser_download_url": "([^"]+)".*/\1/p' \
  | grep -E "/kekkai-${RELEASE_OS}-${RELEASE_ARCH}$|/kekkai$" \
  | head -n 1)

if [ -z "$ASSET_URL" ]; then
  printf '%s\n' "No $RELEASE_OS $RELEASE_ARCH binary was found in the latest GitHub release." >&2
  exit 1
fi

TMP_FILE=$(mktemp "${TMPDIR:-/tmp}/kekkai.XXXXXX")
trap 'rm -f "$TMP_FILE"' EXIT HUP INT TERM
curl -fsSL -H 'User-Agent: kekkai-installer' "$ASSET_URL" -o "$TMP_FILE"

TARGET_DIR='/usr/local/bin'
if [ -d "$TARGET_DIR" ] && [ -w "$TARGET_DIR" ]; then
  install -m 0755 "$TMP_FILE" "$TARGET_DIR/kekkai"
elif command -v sudo >/dev/null 2>&1; then
  sudo mkdir -p "$TARGET_DIR"
  sudo install -m 0755 "$TMP_FILE" "$TARGET_DIR/kekkai"
else
  TARGET_DIR="$HOME/.local/bin"
fi

if [ "$TARGET_DIR" = "$HOME/.local/bin" ]; then
  mkdir -p "$TARGET_DIR"
  install -m 0755 "$TMP_FILE" "$TARGET_DIR/kekkai"
  case "${SHELL:-}" in
    */zsh) PROFILE="$HOME/.zshrc" ;;
    *) PROFILE="$HOME/.bashrc" ;;
  esac
  PATH_LINE='export PATH="$HOME/.local/bin:$PATH"'
  if [ ! -f "$PROFILE" ] || ! grep -Fqx "$PATH_LINE" "$PROFILE"; then
    printf '\n%s\n' "$PATH_LINE" >> "$PROFILE"
  fi
  export PATH="$TARGET_DIR:$PATH"
fi

printf '%s\n' "Kekkai successfully installed! Type 'kekkai' to get started."
