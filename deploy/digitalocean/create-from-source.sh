#!/usr/bin/env bash
# Provision a DigitalOcean droplet that builds qooim from source
# (cloud-init git clone). Requires the repo to be public, which it is
# as of 2026-05-06.
#
# Compared to ./create.sh (the binary-upload flow):
#   + No local toolchain required — anywhere with curl + ssh suffices.
#   + One self-contained cloud-init script; nothing to scp.
#   - Slower bring-up (~6-10 min vs ~1-2 min) — Go install + git clone
#     + `go build -p 1` happen on a 1vCPU droplet.
#   - 1GB droplets OOM during `go build`; cloud-init unconditionally
#     creates a /swapfile of 2 GiB to cover the spike.
#   - Sensitive env (DSN, JWT secret) lives in cloud-init user-data,
#     visible from the droplet's metadata service. For a long-lived
#     install prefer ./create.sh + a separate secret-management step.
#
# Reads (env):
#   DOAPI                  DO API token
#   QOOIM_DB_DSN           PG connection string (postgresql://...)
#   QOOIM_JWT_SECRET       prod JWT secret (random per droplet recommended)
#   GIT_REF                optional, default = main
#   DO_REGION              optional, default = sgp1
#   DO_SIZE                optional, default = s-1vcpu-1gb
#   DO_IMAGE               optional, default = ubuntu-24-04-x64
#   DO_PORT                optional, default = 80
#
# Emits whitelisted KEY=VALUE lines on stdout — pair with `eval`.
set -euo pipefail

: "${DOAPI:?need DOAPI}"
: "${QOOIM_DB_DSN:?need QOOIM_DB_DSN}"
: "${QOOIM_JWT_SECRET:?need QOOIM_JWT_SECRET}"

GIT_REF="${GIT_REF:-main}"
DO_REGION="${DO_REGION:-sgp1}"
DO_SIZE="${DO_SIZE:-s-1vcpu-1gb}"
DO_IMAGE="${DO_IMAGE:-ubuntu-24-04-x64}"
DO_PORT="${DO_PORT:-80}"

DO=https://api.digitalocean.com/v2
H_AUTH="Authorization: Bearer $DOAPI"
H_JSON="Content-Type: application/json"

err() { echo "create-from-source.sh: $*" >&2; exit 1; }

# 1. Ephemeral SSH keypair so `destroy.sh` can clean up afterwards.
mkdir -p /tmp/qooim-do
chmod 700 /tmp/qooim-do
ssh-keygen -t ed25519 -N "" -C "qooim-smoke-$(date +%s)" -f /tmp/qooim-do/id >/dev/null
PUBKEY=$(cat /tmp/qooim-do/id.pub)
KEYNAME="qooim-smoke-$(date +%s)-$RANDOM"

SSH_KEY_ID=$(curl -fsS -X POST "$DO/account/keys" \
  -H "$H_AUTH" -H "$H_JSON" \
  -d "{\"name\":\"$KEYNAME\",\"public_key\":\"$PUBKEY\"}" \
  | python3 -c 'import sys,json;print(json.load(sys.stdin)["ssh_key"]["id"])') \
  || err "register ssh key failed"

GO_VERSION="1.26.0"
USERDATA=$(cat <<EOF
#cloud-config
package_update: true
package_upgrade: false
packages:
  - git
  - wget
  - ca-certificates
write_files:
  - path: /opt/qooim/install.sh
    permissions: '0755'
    content: |
      #!/bin/bash
      # cloud-init runs us as root with a stripped env: HOME is unset
      # and Go 1.26 refuses to use the module cache without it. Set it
      # explicitly. (Verified on a fresh s-1vcpu-1gb droplet 2026-05-06.)
      export HOME=/root
      set -euxo pipefail
      # 1GB droplets OOM during `go build` (peak ~1.3GB across goccy
      # /xuri/excelize/etc.). 2GB swap covers the spike at the cost of
      # ~30s extra build time.
      if ! swapon --show | grep -q '/swapfile'; then
        fallocate -l 2G /swapfile
        chmod 600 /swapfile
        mkswap /swapfile
        swapon /swapfile
      fi
      cd /tmp
      wget -qO go.tgz https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz
      rm -rf /usr/local/go
      tar -C /usr/local -xzf go.tgz
      ln -sf /usr/local/go/bin/go /usr/local/bin/go
      git clone --depth 1 -b ${GIT_REF} https://github.com/web-casa/qooim /opt/qooim/src
      cd /opt/qooim/src
      # -p 1 caps parallel build steps to one — adds time, halves peak
      # RSS so the swap window is short.
      go build -p 1 -o /usr/local/bin/qooim-server ./cmd/server
      QOOIM_DB_DSN='${QOOIM_DB_DSN}' \
        go run github.com/pressly/goose/v3/cmd/goose@v3.27.1 \
          -dir migrations postgres "\$QOOIM_DB_DSN" up
      mkdir -p /var/lib/qooim/storage
      cat >/etc/systemd/system/qooim.service <<UNIT
      [Unit]
      Description=Qoo.IM
      After=network-online.target
      Wants=network-online.target
      [Service]
      Environment=QOOIM_APP_ENV=prod
      Environment=QOOIM_HTTP_ADDR=:${DO_PORT}
      Environment=QOOIM_DB_DSN=${QOOIM_DB_DSN}
      Environment=QOOIM_JWT_SECRET=${QOOIM_JWT_SECRET}
      Environment=QOOIM_STORAGE_LOCAL_ROOT=/var/lib/qooim/storage
      ExecStart=/usr/local/bin/qooim-server
      Restart=on-failure
      [Install]
      WantedBy=multi-user.target
      UNIT
      systemctl daemon-reload
      systemctl enable --now qooim
      touch /var/lib/qooim/READY
runcmd:
  - bash /opt/qooim/install.sh 2>&1 | tee /var/log/qooim-install.log || true
EOF
)
USERDATA_JSON=$(python3 -c 'import json,sys;print(json.dumps(sys.stdin.read()))' <<<"$USERDATA")
DNAME="qooim-smoke-$(date +%s)"

DROPLET_ID=$(curl -fsS -X POST "$DO/droplets" \
  -H "$H_AUTH" -H "$H_JSON" \
  -d "{
    \"name\":\"$DNAME\",
    \"region\":\"$DO_REGION\",
    \"size\":\"$DO_SIZE\",
    \"image\":\"$DO_IMAGE\",
    \"ssh_keys\":[$SSH_KEY_ID],
    \"user_data\":$USERDATA_JSON,
    \"tags\":[\"qooim-smoke\"],
    \"with_droplet_agent\":false,
    \"ipv6\":false
  }" | python3 -c 'import sys,json;print(json.load(sys.stdin)["droplet"]["id"])') \
  || err "droplet create failed"

# 2. Poll for active + public IP.
echo "create-from-source.sh: droplet $DROPLET_ID provisioning…" >&2
IP=""
for i in $(seq 1 60); do
  sleep 5
  D=$(curl -fsS -H "$H_AUTH" "$DO/droplets/$DROPLET_ID")
  STATUS=$(echo "$D" | python3 -c 'import sys,json;print(json.load(sys.stdin)["droplet"]["status"])')
  IP=$(echo "$D" | python3 -c 'import sys,json;d=json.load(sys.stdin)["droplet"];nets=d["networks"]["v4"];print(next((n["ip_address"] for n in nets if n["type"]=="public"), ""))')
  if [ "$STATUS" = "active" ] && [ -n "$IP" ]; then break; fi
done
[ -n "$IP" ] || err "droplet didn't get a public IP within 5min"

echo "DROPLET_ID=$DROPLET_ID"
echo "DROPLET_IP=$IP"
echo "SSH_KEY_ID=$SSH_KEY_ID"
echo "SSH_KEY_PATH=/tmp/qooim-do/id"

# 3. Wait for /healthz on $DO_PORT. cloud-init clones + builds + migrates;
#    on s-1vcpu-1gb that's typically 3-6 min. Bumped from ./create.sh's
#    cap accordingly.
echo "create-from-source.sh: waiting for cloud-init build + /healthz…" >&2
for i in $(seq 1 120); do
  if curl -fsS --max-time 3 "http://$IP:${DO_PORT}/healthz" >/dev/null 2>&1; then
    echo "create-from-source.sh: /healthz up after ~$((i*5))s" >&2
    exit 0
  fi
  sleep 5
done
err "/healthz didn't come up in 10min — droplet $DROPLET_ID still alive at $IP"
