#!/usr/bin/env bash
# Provision a small DigitalOcean droplet, scp a pre-built qooim-server
# binary up, run goose migrations against the DB, install a systemd
# unit, and wait for /healthz to return 200.
#
# This flow does NOT rely on the GitHub repo being public — we ship a
# locally built linux/amd64 binary instead. The earlier autonomous run
# discovered the repo was private, which broke a pure cloud-init
# `git clone` approach.
#
# Reads (env):
#   DOAPI                  DO API token
#   QOOIM_DB_DSN           PG connection string (postgresql://...)
#   QOOIM_JWT_SECRET       prod JWT secret (random per droplet recommended)
#   QOOIM_BINARY           optional, default = ./bin/qooim-server (linux/amd64)
#   GIT_REPO               optional, default = path to migrations/
#   DO_REGION              optional, default = sgp1
#   DO_SIZE                optional, default = s-1vcpu-1gb
#   DO_IMAGE               optional, default = ubuntu-24-04-x64
#   DO_PORT                optional, default = 80
#
# Emits to stdout (one per line):
#   DROPLET_ID=<n>
#   DROPLET_IP=<ip>
#   SSH_KEY_ID=<n>
#   SSH_KEY_PATH=/tmp/qooim-do/id   (kept for the destroy script)
set -euo pipefail

: "${DOAPI:?need DOAPI}"
: "${QOOIM_DB_DSN:?need QOOIM_DB_DSN}"
: "${QOOIM_JWT_SECRET:?need QOOIM_JWT_SECRET}"

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
QOOIM_BINARY="${QOOIM_BINARY:-$REPO_ROOT/bin/qooim-server-linux-amd64}"
MIGRATIONS_DIR="${MIGRATIONS_DIR:-$REPO_ROOT/migrations}"
DO_REGION="${DO_REGION:-sgp1}"
DO_SIZE="${DO_SIZE:-s-1vcpu-1gb}"
DO_IMAGE="${DO_IMAGE:-ubuntu-24-04-x64}"
DO_PORT="${DO_PORT:-80}"

DO=https://api.digitalocean.com/v2
H_AUTH="Authorization: Bearer $DOAPI"
H_JSON="Content-Type: application/json"

err() { echo "create.sh: $*" >&2; exit 1; }

# 0. Build the linux/amd64 binary if it isn't there yet.
if [ ! -f "$QOOIM_BINARY" ]; then
  echo "create.sh: building $QOOIM_BINARY (linux/amd64)" >&2
  ( cd "$REPO_ROOT" && GOOS=linux GOARCH=amd64 go build -o "$QOOIM_BINARY" ./cmd/server ) \
    || err "go build failed"
fi
[ -f "$MIGRATIONS_DIR/00001_schema.sql" ] || err "missing migrations at $MIGRATIONS_DIR"

# 1. Ephemeral SSH keypair stored under /tmp/qooim-do so destroy.sh can
#    finish even after the smoke harness shell has gone.
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

# 2. Create droplet. cloud-init only needs to install Go (for goose) +
#    open the firewall hole; the binary itself is uploaded later.
USERDATA=$(cat <<'EOF'
#cloud-config
package_update: true
package_upgrade: false
packages:
  - wget
  - ca-certificates
runcmd:
  - mkdir -p /opt/qooim /var/lib/qooim/storage
  - wget -qO /tmp/go.tgz https://go.dev/dl/go1.26.0.linux-amd64.tar.gz
  - rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tgz
  - ln -sf /usr/local/go/bin/go /usr/local/bin/go
  - touch /var/lib/qooim/cloud-init-done
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

# 3. Poll for active + public IP.
echo "create.sh: droplet $DROPLET_ID provisioning…" >&2
IP=""
for i in $(seq 1 60); do
  sleep 5
  D=$(curl -fsS -H "$H_AUTH" "$DO/droplets/$DROPLET_ID")
  STATUS=$(echo "$D" | python3 -c 'import sys,json;print(json.load(sys.stdin)["droplet"]["status"])')
  IP=$(echo "$D" | python3 -c 'import sys,json;d=json.load(sys.stdin)["droplet"];nets=d["networks"]["v4"];print(next((n["ip_address"] for n in nets if n["type"]=="public"), ""))')
  if [ "$STATUS" = "active" ] && [ -n "$IP" ]; then
    break
  fi
done
[ -n "$IP" ] || err "droplet didn't get a public IP within 5min"

# 4. Wait for cloud-init to finish (Go install + apt) — usually 1-2 min.
echo "create.sh: waiting for cloud-init (Go install)…" >&2
SSH="ssh -i /tmp/qooim-do/id -o StrictHostKeyChecking=no -o ConnectTimeout=8 -o LogLevel=ERROR root@$IP"
for i in $(seq 1 60); do
  sleep 5
  if $SSH 'test -f /var/lib/qooim/cloud-init-done' 2>/dev/null; then
    break
  fi
done

# 5. scp the binary + migrations and install the systemd unit.
echo "create.sh: uploading binary + migrations…" >&2
SCP="scp -i /tmp/qooim-do/id -o StrictHostKeyChecking=no -o LogLevel=ERROR"
$SCP "$QOOIM_BINARY" root@$IP:/usr/local/bin/qooim-server
$SCP -r "$MIGRATIONS_DIR" root@$IP:/opt/qooim/migrations

$SSH bash -s <<UNIT_INSTALL
set -euo pipefail
chmod +x /usr/local/bin/qooim-server
mkdir -p /var/lib/qooim/storage

# Apply migrations against the configured DB.
QOOIM_DB_DSN='${QOOIM_DB_DSN}' \
  /usr/local/bin/go run github.com/pressly/goose/v3/cmd/goose@v3.27.1 \
    -dir /opt/qooim/migrations postgres "\$QOOIM_DB_DSN" up

cat >/etc/systemd/system/qooim.service <<'UNIT'
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
UNIT_INSTALL

# 6. Wait for /healthz over the public IP.
echo "create.sh: waiting for /healthz on http://$IP:$DO_PORT/healthz" >&2
for i in $(seq 1 60); do
  if curl -fsS --max-time 3 "http://$IP:${DO_PORT}/healthz" >/dev/null 2>&1; then
    echo "DROPLET_ID=$DROPLET_ID"
    echo "DROPLET_IP=$IP"
    echo "SSH_KEY_ID=$SSH_KEY_ID"
    echo "SSH_KEY_PATH=/tmp/qooim-do/id"
    echo "create.sh: /healthz up after ~$((i*5))s" >&2
    exit 0
  fi
  sleep 5
done
err "/healthz didn't come up in 5min — droplet $DROPLET_ID still alive at $IP"
