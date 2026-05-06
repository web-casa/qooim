# DigitalOcean smoke deploy

A small, scripted "spin up a real VPS, deploy, smoke-test, tear down"
flow used during the autonomous P0..P5 run. The same scripts double as
the **canonical recipe for a single-droplet Qoo.IM install**.

Cost note: each `create.sh` invocation provisions an `s-1vcpu-1gb`
droplet at ~$0.009/hour (~$0.0002/minute). A typical full run is well
under 5 minutes and costs a fraction of a cent. Always pair with
`destroy.sh`.

## Prerequisites

- `DOAPI` env var with a DO API token (Bearer, `read+write` on droplets +
  ssh keys).
- `QOOIM_DB_DSN` pointing at a reachable PostgreSQL 18 (we recommend a
  managed PG so the droplet is stateless and disposable).
- `QOOIM_JWT_SECRET` — a high-entropy string, e.g. `openssl rand -hex 32`.
- Local `go` toolchain to cross-compile the linux/amd64 binary on demand
  (the script does this automatically if `bin/qooim-server-linux-amd64`
  is missing).

## Files

| File                      | Purpose                                                                                                            |
| ------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| `create.sh`               | **Default**. Cross-compile locally, scp binary + migrations, install systemd unit, wait for /healthz.              |
| `create-from-source.sh`   | Alternative. cloud-init `git clone` + Go build on the droplet. No local toolchain required, but slower bring-up.   |
| `smoke.sh`                | Walk the public API surface against a running instance. Pass `DROPLET_IP=...`.                                     |
| `destroy.sh`              | Delete the droplet + the ephemeral ssh key from DO.                                                                |

### Which create script to use?

| You want…                                          | Use                       |
| -------------------------------------------------- | ------------------------- |
| Fastest bring-up (~1-2 min) for repeated tests     | `create.sh`               |
| To deploy from any machine without `go` installed  | `create-from-source.sh`   |
| To keep the DSN/secret out of cloud-init metadata  | `create.sh`               |
| The simplest one-shot recipe (everything on droplet) | `create-from-source.sh` |

## End-to-end

```bash
export DOAPI='dop_v1_xxx'
export QOOIM_DB_DSN='postgresql://user:pass@host:5432/qooim?sslmode=disable'
export QOOIM_JWT_SECRET="$(openssl rand -hex 32)"

eval "$(./deploy/digitalocean/create.sh | tee /tmp/qooim-do.env)"
DROPLET_IP=$DROPLET_IP ./deploy/digitalocean/smoke.sh
DROPLET_ID=$DROPLET_ID SSH_KEY_ID=$SSH_KEY_ID DOAPI=$DOAPI \
  ./deploy/digitalocean/destroy.sh
```

`eval` is fine here because `create.sh` only emits whitelisted
`KEY=VALUE` lines.

## Implementation notes (autonomous run findings)

The first autonomous attempt baked `git clone https://github.com/web-casa/qooim`
straight into cloud-init. It failed at the time because the repo was
private. The fix was to ship a pre-built linux/amd64 binary over scp,
which is what `create.sh` does today.

The repo went public on 2026-05-06, so the original "all-in-cloud-init"
flow is back on the table; it's preserved as `create-from-source.sh`.
`create.sh` remains the default because it's faster and keeps the DSN
+ JWT secret off the metadata service.

Other fixes folded in:

- The smoke pass surfaced **0 functional issues** against the running
  binary; the entire P0..P5 surface (login → /me → CRUD → public
  survey → answer → report → xlsx → AI 404) responded as expected.
- `validateConfig` already refuses to start in `env=prod` without a
  real `QOOIM_JWT_SECRET`, so the systemd unit's hard-coded production
  envs are checked before the listener binds.

## Tunables

| Env             | Default                                  | Notes                                |
| --------------- | ---------------------------------------- | ------------------------------------ |
| `DO_REGION`     | `sgp1`                                   | Singapore                            |
| `DO_SIZE`       | `s-1vcpu-1gb`                            | $0.009/h, fine for smoke/dev         |
| `DO_IMAGE`      | `ubuntu-24-04-x64`                       | Latest Ubuntu LTS                    |
| `DO_PORT`       | `80`                                     | Listening port                        |
| `QOOIM_BINARY`  | `bin/qooim-server-linux-amd64`           | Cross-compiled on demand if missing  |
| `MIGRATIONS_DIR`| `migrations/`                            | scp'd up so goose can run remote     |

## Stage-2 hardening (not done here)

For a long-lived deploy you'd want, at minimum:

- TLS via Caddy or a stand-alone certbot loop.
- A non-root systemd User= and ProtectSystem= hardening.
- Outbound network restrictions.
- Backups of the storage volume (or use S3 instead of local).
- Rate limiting on `/api/auth/login` and `/api/survey/*` (none today).
