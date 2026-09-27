#!/bin/sh
# Build and install ProPresenter Toolkit as a systemd service on Debian
# (e.g. a Proxmox LXC). Run as root from a checkout of this repository:
#
#     sh deploy/install.sh
#
# Safe to re-run: to update, `git pull` and run it again. Your settings in
# /etc/propresenter-toolkit.env and your styles are kept.
set -eu

APP=propresenter-toolkit
INSTALL_DIR=/opt/$APP
DATA_DIR=/var/lib/$APP
ENV_FILE=/etc/$APP.env
UNIT_FILE=/etc/systemd/system/$APP.service
MIN_GO=1.24.1

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"

say() { printf '\n==> %s\n' "$*"; }

if [ "$(id -u)" -ne 0 ]; then
  echo "Run this as root (e.g. sudo sh deploy/install.sh)." >&2
  exit 1
fi
if [ ! -f "$REPO_DIR/go.mod" ]; then
  echo "Run this from inside a checkout of the repository." >&2
  exit 1
fi

say "Installing packages"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq ca-certificates curl git >/dev/null

# --- Go toolchain -----------------------------------------------------------
# Use Debian's Go if it's new enough (Debian 13 ships 1.24), otherwise
# install the official release into /usr/local/go.
version_ok() { # version_ok 1.24.4 -> true if >= MIN_GO
  [ "$(printf '%s\n%s\n' "$MIN_GO" "$1" | sort -V | head -n1)" = "$MIN_GO" ]
}
go_version() {
  "$1" env GOVERSION 2>/dev/null | sed 's/^go//'
}
find_go() {
  for g in /usr/local/go/bin/go "$(command -v go 2>/dev/null || true)" /usr/lib/go/bin/go; do
    [ -n "$g" ] && [ -x "$g" ] || continue
    v="$(go_version "$g")"
    if [ -n "$v" ] && version_ok "$v"; then
      echo "$g"
      return 0
    fi
  done
  return 1
}

GO="$(find_go || true)"
if [ -z "$GO" ]; then
  say "Trying Debian's Go package"
  apt-get install -y -qq golang-go >/dev/null 2>&1 || true
  GO="$(find_go || true)"
fi
if [ -z "$GO" ]; then
  arch="$(dpkg --print-architecture)"
  case "$arch" in amd64|arm64) ;; *) echo "Unsupported architecture: $arch" >&2; exit 1 ;; esac
  latest="$(curl -fsSL 'https://go.dev/VERSION?m=text' | head -n1)"
  say "Installing $latest from go.dev"
  curl -fsSL "https://go.dev/dl/$latest.linux-$arch.tar.gz" -o /tmp/go.tgz
  rm -rf /usr/local/go
  tar -C /usr/local -xzf /tmp/go.tgz
  rm -f /tmp/go.tgz
  GO=/usr/local/go/bin/go
fi
echo "Using $("$GO" version)"

# --- build ------------------------------------------------------------------
say "Building"
VERSION="$(git -c safe.directory="$REPO_DIR" -C "$REPO_DIR" describe --tags --always --dirty 2>/dev/null || echo dev)"
mkdir -p "$INSTALL_DIR"
(
  cd "$REPO_DIR"
  CGO_ENABLED=0 GOTOOLCHAIN=local "$GO" build -trimpath -buildvcs=false \
    -ldflags "-s -w -X main.version=$VERSION" \
    -o "$INSTALL_DIR/pptoolkit.new" ./cmd/pptoolkit
)
mv "$INSTALL_DIR/pptoolkit.new" "$INSTALL_DIR/pptoolkit"
echo "Built $VERSION -> $INSTALL_DIR/pptoolkit"

# --- user, data, settings -----------------------------------------------------
if ! id "$APP" >/dev/null 2>&1; then
  useradd --system --home-dir "$DATA_DIR" --no-create-home --shell /usr/sbin/nologin "$APP"
fi
install -d -o "$APP" -g "$APP" -m 0750 "$DATA_DIR" "$DATA_DIR/styles"

if [ ! -f "$ENV_FILE" ]; then
  say "Writing $ENV_FILE"
  cat >"$ENV_FILE" <<EOF
# ProPresenter Toolkit settings. After editing:
#   systemctl restart $APP

# Address and port to listen on.
PPT_ADDR=:5000

# Where style profiles are kept (back this folder up).
PPT_STYLES_DIR=$DATA_DIR/styles

# Require a login, as user:password. Leave empty for no login
# (only do that on a trusted network).
PPT_AUTH=
EOF
  chmod 0640 "$ENV_FILE"
  chgrp "$APP" "$ENV_FILE"
fi

# --- service ------------------------------------------------------------------
say "Installing systemd service"
cat >"$UNIT_FILE" <<EOF
[Unit]
Description=ProPresenter Toolkit (PDF to ProPresenter .pro)
After=network-online.target
Wants=network-online.target

[Service]
User=$APP
Group=$APP
EnvironmentFile=$ENV_FILE
ExecStart=$INSTALL_DIR/pptoolkit
Restart=on-failure
RestartSec=3

NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=$DATA_DIR

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable "$APP" >/dev/null 2>&1
systemctl restart "$APP"
sleep 1

if systemctl is-active --quiet "$APP"; then
  port="$(sed -n 's/^PPT_ADDR=.*:\([0-9]*\)$/\1/p' "$ENV_FILE")"
  ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
  say "Done - running $VERSION at http://${ip:-<this-container-ip>}:${port:-5000}"
  echo "Settings: $ENV_FILE"
  echo "Styles:   $DATA_DIR/styles"
  echo "Logs:     journalctl -u $APP -f"
else
  echo "The service didn't start. See: journalctl -u $APP -e" >&2
  exit 1
fi
