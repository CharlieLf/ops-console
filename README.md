# Chelops Monitoring

One process for this Docker host: stack control, live metrics, checks, and auto-prune.

| Page | Port path | Role |
|---|---|---|
| Dashboard | / | host facts, storage, findings |
| Stacks | /stacks | compose up/down/edit, container logs |
| Metrics | /metrics | host + container CPU/RAM |
| Checks | /checks | reliability / security / storage rules |
| Resources | /resources | images, volumes, networks |
| Auto-prune | /prune | scheduled cleanup |

Open http://localhost:7070

## Why one service

Dockge and Beszel were separate UIs on top of the same socket. Chelops folds the
useful bits into the existing Go console so RAM stays low and there is one place
to operate the host.

## Stack control

Per Compose project: edit the compose file, `up` / `down` / `restart` / `pull`,
plus per-container start/stop/restart and log tails. Needs `/opt/stacks` mounted
read-write at the same path (so label paths match).

## Metrics

Background sampler (~15s) stores ~30 minutes of host CPU/RAM in memory and the
latest per-container docker stats. No separate agent, no database.

## Auto-prune

Same as before: interval / daily / weekly, keep patterns, dry-run, history.
Label `ops-console.keep=true` exempts an object.

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `LISTEN` | `:7070` | bind address |
| `DATA_DIR` | `/app/data` | `ops-console.json` |
| `STATIC_DIR` | `/app/static` | built UI |
| `DOCKER_SOCKET` | `/var/run/docker.sock` | Engine API |
| `HOST_STACKS_DIR` | `/opt/stacks` | compose roots |
| `HOST_PROC` | `/host/proc` | host meminfo/stat (bind-mount host `/proc`) |
| `API_KEY` | empty | when set, `/api/*` needs `X-API-Key` |
| `TZ` | `Asia/Jakarta` | timezone |

## Run

```bash
docker network create proxy   # if needed
cd /opt/stacks/ops-console
docker compose up -d --build
```

Docker socket = root on the host. Do not publish 7070 to the internet; use
Tailscale or NPM + `API_KEY`.

## Stack

- Single Alpine image: Go binary + React UI + `docker compose` CLI
- `mem_limit: 96m`
- External `proxy` network

## Local dev

```bash
cd backend
DATA_DIR=../data STATIC_DIR=../frontend/dist HOST_STACKS_DIR=/opt/stacks HOST_PROC=/proc LISTEN=:7070 go run .
```

```bash
cd frontend && npm install && npm run dev
```

```bash
cd backend && go test ./...
```
