#!/bin/sh
set -e

REPO="operator-kit/ghapp-cli"
BINARY="ghapp"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

# Detect OS
case "$(uname -s)" in
  Linux*)  OS="linux" ;;
  Darwin*) OS="darwin" ;;
  *)       echo "Unsupported OS: $(uname -s)"; exit 1 ;;
esac

# Detect arch
case "$(uname -m)" in
  x86_64|amd64)  ARCH="amd64" ;;
  arm64|aarch64)  ARCH="arm64" ;;
  *)              echo "Unsupported architecture: $(uname -m)"; exit 1 ;;
esac

# Resolve version
if [ -z "$GHAPP_VERSION" ]; then
  GHAPP_VERSION=$(curl -sI "https://github.com/${REPO}/releases/latest" \
    | grep -i "^location:" \
    | sed 's|.*/tag/||' \
    | tr -d '\r\n')

  if [ -z "$GHAPP_VERSION" ]; then
    echo "Error: could not determine latest version. Set GHAPP_VERSION manually."
    exit 1
  fi
fi

VERSION_NUM="${GHAPP_VERSION#v}"
ARCHIVE="ghapp_${VERSION_NUM}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${GHAPP_VERSION}/${ARCHIVE}"
CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${GHAPP_VERSION}/checksums.txt"

echo "Installing ${BINARY} ${GHAPP_VERSION} (${OS}/${ARCH})..."

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

# Download archive + checksums
curl -sL "$URL" -o "$TMPDIR/$ARCHIVE"
curl -sL "$CHECKSUMS_URL" -o "$TMPDIR/checksums.txt"

# Verify checksum
cd "$TMPDIR"
if command -v sha256sum >/dev/null 2>&1; then
  grep "$ARCHIVE" checksums.txt | sha256sum -c --quiet
elif command -v shasum >/dev/null 2>&1; then
  grep "$ARCHIVE" checksums.txt | shasum -a 256 -c --quiet
else
  echo "Warning: no sha256sum or shasum found, skipping checksum verification"
fi

# Extract and install
tar xzf "$ARCHIVE" "$BINARY" "ghapp-gh"
install -d "$INSTALL_DIR"
install "$BINARY" "$INSTALL_DIR/$BINARY"
install "ghapp-gh" "$INSTALL_DIR/ghapp-gh"

echo "Installed ${BINARY} and ghapp-gh to ${INSTALL_DIR}"
"$INSTALL_DIR/$BINARY" version

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    # Detect user's shell rc file
    case "$(basename "$SHELL")" in
      zsh)  RC_FILE="$HOME/.zshrc" ;;
      fish) RC_FILE="$HOME/.config/fish/config.fish" ;;
      *)    RC_FILE="$HOME/.bashrc" ;;
    esac
    echo ""
    echo "WARNING: $INSTALL_DIR is not in your PATH. Run:"
    case "$(basename "$SHELL")" in
      fish)
        echo "  echo 'fish_add_path $INSTALL_DIR' >> $RC_FILE && source $RC_FILE"
        ;;
      *)
        echo "  echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> $RC_FILE && source $RC_FILE"
        ;;
    esac
    ;;
esac
