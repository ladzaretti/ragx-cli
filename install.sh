#!/bin/bash
set -euo pipefail

case "$OSTYPE" in
linux-gnu | darwin) ;;
*)
    echo "unsupported OS: $OS" >&2
    exit 1
    ;;
esac

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
    echo "Usage: $0 [version]"
    echo
    echo "If no version is provided, the latest release will be installed."
    echo
    echo "Examples:"
    echo "  $0           # installs the latest version"
    echo "  $0 0.2.0     # installs version 0.2.0"
    exit 0
fi

OS=$(uname | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
INSTALL_DIR=/usr/local/bin

case "$ARCH" in
x86_64) ARCH="amd64" ;;
aarch64 | arm64) ARCH="arm64" ;;
i386 | i686) ARCH="386" ;;
*)
    echo "unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

VERSION=${1:-latest}
TMP_DIR=$(mktemp -d)

trap 'rm -rf "$TMP_DIR"' EXIT

if [ "$VERSION" = "latest" ]; then
    VERSION=$(curl -s https://api.github.com/repos/ladzaretti/ragx-cli/releases/latest |
        grep '"tag_name":' |
        sed -E 's/.*"v([^"]+)".*/\1/')
fi

if [[ -z "$VERSION" ]]; then
    echo "failed to determine version" >&2
    exit 1
fi

echo "preparing install:"
echo "  OS      : $OS"
echo "  ARCH    : $ARCH"
echo "  VERSION : $VERSION"

TARBALL=ragx_${VERSION}_${OS}_${ARCH}.tar.gz
DEST="$TMP_DIR"/"$TARBALL"
URL="https://github.com/ladzaretti/ragx-cli/releases/download/v$VERSION/$TARBALL"

printf "\nresolved release archive url: \"%s\"\n\n" "$URL"

printf "downloading release archive..."
curl -sSL --fail-with-body -o "$DEST" "$URL"

printf "\t\t\tok\n"

printf "extracting archive..."
tar -xz -C "$TMP_DIR" -f "$DEST"

printf "\t\t\t\tok\n"

cd "$TMP_DIR/ragx_${VERSION}_${OS}_${ARCH}"

if [[ ! -x ragx ]]; then
    echo "ragx not found or not executable in $PWD" >&2
    exit 1
fi

printf "installing binaries to %s..." "$INSTALL_DIR"

sudo install -m 0755 ragx "$INSTALL_DIR/ragx.new"
sudo mv -f "$INSTALL_DIR/ragx.new" "$INSTALL_DIR/ragx"

printf "\tok\n"
