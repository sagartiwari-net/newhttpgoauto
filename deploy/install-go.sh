#!/bin/bash
# Install Go 1.22 on Ubuntu/Linux (run as root)
set -e

GO_VERSION="1.22.10"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  GO_ARCH="amd64" ;;
  aarch64) GO_ARCH="arm64" ;;
  *) echo "Unsupported arch: $ARCH"; exit 1 ;;
esac

TARBALL="go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
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

echo "==> Downloading Go ${GO_VERSION}..."
cd /tmp
wget -q "$URL" -O "$TARBALL"
rm -rf /usr/local/go
tar -C /usr/local -xzf "$TARBALL"
rm -f "$TARBALL"

if ! grep -q '/usr/local/go/bin' /etc/profile.d/go.sh 2>/dev/null; then
  echo 'export PATH=$PATH:/usr/local/go/bin' > /etc/profile.d/go.sh
  chmod +x /etc/profile.d/go.sh
fi
export PATH=$PATH:/usr/local/go/bin

echo "==> Installed: $(go version)"
