#!/bin/sh
# Install the ProPresenter Toolkit web app as a systemd service inside a
# plain Debian/Ubuntu LXC container (no Docker needed).
#
# 1. Put the Linux binary next to this script, named "pptoolkit":
#      - download pptoolkit-linux-amd64 from the GitHub Releases page, or
#      - build it:  GOOS=linux GOARCH=amd64 go build -o deploy/pptoolkit ./cmd/pptoolkit
# 2. Copy this folder into the container and run, as root:
#      sh install-lxc.sh
#
# Optional: PPT_AUTH="user:password" sh install-lxc.sh   to require a login.
set -eu

HERE="$(cd "$(dirname "$0")" && pwd)"
INSTALL_DIR=/opt/propresenter-toolkit
DATA_DIR=/var/lib/propresenter-toolkit/styles

if [ ! -f "$HERE/pptoolkit" ]; then
  echo "Put the Linux 'pptoolkit' binary next to this script first." >&2
  exit 1
fi

id pptoolkit >/dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin pptoolkit
install -d "$INSTALL_DIR"
install -m 0755 "$HERE/pptoolkit" "$INSTALL_DIR/pptoolkit"
install -d -o pptoolkit -g pptoolkit "$DATA_DIR"

cat > /etc/systemd/system/propresenter-toolkit.service <<UNIT
[Unit]
Description=ProPresenter Toolkit (web)
After=network.target

[Service]
User=pptoolkit
Environment=PPT_ADDR=:5000
Environment=PPT_STYLES_DIR=$DATA_DIR
Environment=PPT_AUTH=${PPT_AUTH:-}
ExecStart=$INSTALL_DIR/pptoolkit serve
Restart=on-failure
NoNewPrivileges=true
ProtectSystem=strict
ReadWritePaths=$DATA_DIR

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable --now propresenter-toolkit
echo
echo "Running at http://<this-container-ip>:5000"
echo "Styles folder: $DATA_DIR   (back this up)"
echo "Logs: journalctl -u propresenter-toolkit -f"
