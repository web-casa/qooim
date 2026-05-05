#!/usr/bin/env bash
# Tear down the DO droplet + ssh key created by create.sh.
# Usage: DROPLET_ID=… SSH_KEY_ID=… DOAPI=… ./destroy.sh
set -euo pipefail
: "${DOAPI:?need DOAPI}"
: "${DROPLET_ID:?need DROPLET_ID}"
H_AUTH="Authorization: Bearer $DOAPI"
DO=https://api.digitalocean.com/v2

curl -fsS -X DELETE "$DO/droplets/$DROPLET_ID" -H "$H_AUTH" \
  && echo "destroy.sh: droplet $DROPLET_ID deleted" >&2

if [ -n "${SSH_KEY_ID:-}" ]; then
  curl -fsS -X DELETE "$DO/account/keys/$SSH_KEY_ID" -H "$H_AUTH" \
    && echo "destroy.sh: ssh key $SSH_KEY_ID deleted" >&2
fi
rm -rf /tmp/qooim-do
