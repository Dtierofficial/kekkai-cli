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

# Verify the binary against the release checksum manifest (SHA256SUMS.txt).
# A manifest that cannot be fetched only produces a warning; a fetched
# manifest that does not match is a hard failure.
ASSET_NAME=$(basename "$ASSET_URL")
SUMS_URL="https://github.com/$REPO/releases/latest/download/SHA256SUMS.txt"
if SUMS=$(curl -fsSL -H 'User-Agent: kekkai-installer' "$SUMS_URL" 2>/dev/null); then
  EXPECTED=$(printf '%s\n' "$SUMS" | grep -F "  $ASSET_NAME" | awk '{print $1}' | head -n 1)
  if [ -n "$EXPECTED" ]; then
    ACTUAL=''
    if command -v sha256sum >/dev/null 2>&1; then
      ACTUAL=$(sha256sum "$TMP_FILE" | awk '{print $1}')
    elif command -v shasum >/dev/null 2>&1; then
      ACTUAL=$(shasum -a 256 "$TMP_FILE" | awk '{print $1}')
    else
      printf '%s\n' 'No sha256sum/shasum available; skipping checksum verification.' >&2
    fi
    if [ -n "$ACTUAL" ] && [ "$ACTUAL" != "$EXPECTED" ]; then
      printf '%s\n' 'SHA256 checksum mismatch: the downloaded binary is corrupted or has been tampered with.' >&2
      exit 1
    fi
    if [ -n "$ACTUAL" ]; then
      printf '%s\n' 'Checksum verified.'
    fi
  else
    printf '%s\n' 'Checksum manifest did not contain this asset; skipping verification.' >&2
  fi
else
  printf '%s\n' 'Could not fetch SHA256SUMS.txt; skipping checksum verification.' >&2
fi

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
