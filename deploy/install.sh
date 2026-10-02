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
MIN_GO=1.26.0

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
# Use the system Go if it's new enough, otherwise
# install the official release into /usr/local/go.
version_ok() { # version_ok 1.24.4 -> true if >= MIN_GO
  [ "$(printf '%s\n%s\n' "$MIN_GO" "$1" | sort -V | head -n1)" = "$MIN_GO" ]
}
go_version() {
  # Ask from outside the repo with GOTOOLCHAIN=local: inside it, an older Go
  # would quietly fetch the version go.mod asks for and report that instead.
  (cd / && GOTOOLCHAIN=local "$1" env GOVERSION 2>/dev/null) | sed 's/^go//'
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
install -d -o "$APP" -g "$APP" -m 0750 "$DATA_DIR" "$DATA_DIR/styles" "$DATA_DIR/certs"

tls_settings() {
  cat <<EOF

# --- HTTPS (optional) ---------------------------------------------------------
# To serve https://<domain> with a free, automatically renewed Let's Encrypt
# certificate, set the domain and a Cloudflare API token for its zone
# (Cloudflare dashboard -> My Profile -> API Tokens -> Create Token, with
# permissions Zone / Zone / Read and Zone / DNS / Edit, limited to the zone).
# Plain http on PPT_ADDR then redirects to https.
PPT_DOMAIN=
PPT_CLOUDFLARE_API_TOKEN=
# Optional: an address for Let's Encrypt's certificate expiry notices.
PPT_ACME_EMAIL=
PPT_CERT_DIR=$DATA_DIR/certs
EOF
}

if [ ! -f "$ENV_FILE" ]; then
  say "Writing $ENV_FILE"
  cat >"$ENV_FILE" <<EOF
# ProPresenter Toolkit settings. After editing:
#   systemctl restart $APP

# Address and port to listen on. Port 80 means the address needs no ":port".
PPT_ADDR=:80

# Where style profiles are kept (back this folder up).
PPT_STYLES_DIR=$DATA_DIR/styles

# Require a login, as user:password. Leave empty for no login
# (only do that on a trusted network).
PPT_AUTH=
EOF
  tls_settings >>"$ENV_FILE"
  chmod 0640 "$ENV_FILE"
  chgrp "$APP" "$ENV_FILE"
else
  if grep -qx 'PPT_ADDR=:5000' "$ENV_FILE"; then
    # Older installs defaulted to port 5000; move them to 80. A port you set
    # yourself is left alone.
    say "Moving from port 5000 to 80 (edit PPT_ADDR in $ENV_FILE to change)"
    sed -i 's/^PPT_ADDR=:5000$/PPT_ADDR=:80/' "$ENV_FILE"
    sed -i 's/^# Address and port to listen on\.$/# Address and port to listen on. Port 80 means the address needs no ":port"./' "$ENV_FILE"
  fi
  if ! grep -q '^PPT_DOMAIN=' "$ENV_FILE"; then
    say "Adding the (optional) HTTPS settings to $ENV_FILE"
    tls_settings >>"$ENV_FILE"
  fi
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

# Allow binding ports below 1024 (e.g. 80) without running as root; this
# is the only privilege the service gets.
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
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
  suffix=":${port:-80}"
  [ "$suffix" = ":80" ] && suffix=""
  domain="$(sed -n 's/^PPT_DOMAIN=\(.*\)$/\1/p' "$ENV_FILE" | tr -d ' "')"
  if [ -n "$domain" ]; then
    say "Done - running $VERSION at https://$domain"
    echo "The first start fetches the certificate, which can take a minute or two;"
    echo "watch it with: journalctl -u $APP -f"
  else
    say "Done - running $VERSION at http://${ip:-<this-container-ip>}$suffix"
    echo "(or at the name you gave it in DNS; set PPT_DOMAIN in $ENV_FILE for https)"
  fi
  echo "Settings: $ENV_FILE"
  echo "Styles:   $DATA_DIR/styles"
  echo "Logs:     journalctl -u $APP -f"
else
  echo "The service didn't start. See: journalctl -u $APP -e" >&2
  exit 1
fi
