#!/bin/bash
# Install Go 1.22 on macOS without Homebrew (run in Terminal)
set -e

GO_VERSION="1.22.10"
ARCH="$(uname -m)"
case "$ARCH" in
  arm64)  GO_ARCH="arm64" ;;
  x86_64) GO_ARCH="amd64" ;;
  *) echo "Unsupported Mac arch: $ARCH"; exit 1 ;;
esac

INSTALL_DIR="${HOME}/.local"
GO_ROOT="${INSTALL_DIR}/go"
TARBALL="go${GO_VERSION}.darwin-${GO_ARCH}.tar.gz"
URL="https://go.dev/dl/${TARBALL}"

if command -v go >/dev/null 2>&1; then
  VER="$(go version | awk '{print $3}' | tr -d 'go')"
  MAJOR="${VER%%.*}"
  MINOR="${VER#*.}"; MINOR="${MINOR%%.*}"
  if [ "$MAJOR" -ge 1 ] && [ "$MINOR" -ge 22 ]; then
    echo "Go already installed: $(go version)"
    exit 0
  fi
fi

echo "==> Downloading Go ${GO_VERSION} for macOS ${GO_ARCH}..."
mkdir -p "$INSTALL_DIR"
cd /tmp
curl -fsSL "$URL" -o "$TARBALL"
rm -rf "$GO_ROOT"
tar -C "$INSTALL_DIR" -xzf "$TARBALL"
rm -f "$TARBALL"

PATH_LINE='export PATH="$HOME/.local/go/bin:$PATH"'
if ! grep -q '.local/go/bin' "$HOME/.zshrc" 2>/dev/null; then
  echo "" >> "$HOME/.zshrc"
  echo "# Go (GoHttpAuto worker)" >> "$HOME/.zshrc"
  echo "$PATH_LINE" >> "$HOME/.zshrc"
fi
export PATH="$HOME/.local/go/bin:$PATH"

echo "==> Installed: $(go version)"
echo "==> PATH updated in ~/.zshrc — run: source ~/.zshrc"
