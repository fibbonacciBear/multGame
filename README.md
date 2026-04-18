# AstroDrift

AstroDrift is a multiplayer browser arena game with a React/Vite client, Go-based backend services, Redis-backed matchmaking and leaderboard state, and Kubernetes manifests for deployment.

Play the live version at [astrodrift.io](https://astrodrift.io).

## What This Repo Contains

- `client`: React + TypeScript frontend rendered with Vite.
- `server`: real-time game simulation and gameplay WebSocket server.
- `api`: matchmaking, token issuance, and leaderboard API.
- `ws-router`: WebSocket edge router used to forward players to the correct game pod in multi-pod deployments.
- `config`: shared local game-server tuning plus local override templates.
- `k8s`: base manifests, overlays, monitoring, and Argo CD resources.

## Gameplay At A Glance

- Mouse position controls movement direction and thrust strength.
- Hold mouse button or press `Space` to fire.
- Players collect mass, fight other pilots, respawn during matches, and compete on a persistent leaderboard.
- Matches run with an active phase plus intermission, and the frontend shows HUD, kill feed, minimap, and scoreboard overlays.

## Architecture

For local development, the default path is:

1. The browser loads the React client on port `5173`.
2. The client calls the `api` service on port `8081` to join matchmaking.
3. The API returns a signed WebSocket route for the game server.
4. The client connects to the game server WebSocket on port `8080`.
5. Redis stores lobby registry data and leaderboard state.

In production, the `ws-router` sits in front of game pods so the API can hand clients a stable WebSocket entrypoint while routing each player to the correct lobby backend.

## Quick Start

### Prerequisites

- Docker and Docker Compose
- `make`

### Run Everything Locally

```bash
make run
```

This starts:

- `web-client` at [http://localhost:5173](http://localhost:5173)
- `api-server` at [http://localhost:8081](http://localhost:8081)
- `game-server` at `ws://localhost:8080/ws`
- `redis` at `localhost:6379`

Stop the stack with:

```bash
make compose-down
```

## Local Configuration

The game server reads:

- `config/game-server.env`: committed baseline values used by local Compose
- `config/game-server.local.env`: optional local-only overrides
- `config/game-server.local.env.example`: template for common overrides

To customize local tuning:

```bash
cp config/game-server.local.env.example config/game-server.local.env
```

Then edit only the keys you want to override.

Common knobs include:

- tick and snapshot rates
- player caps
- gravity, drag, and terminal speed
- projectile damage and cooldown
- match, respawn, and intermission timing
- bot difficulty distribution

## Running Services Individually

If you want a non-Docker workflow, install each service's dependencies and run them separately.

### Client

```bash
cd client
npm ci
npm run dev
```

### API

```bash
cd api
go run ./cmd/api
```

### Game Server

```bash
cd server
go run ./cmd/gameserver
```

### WebSocket Router

```bash
cd ws-router
go run ./cmd/router
```

### Redis

Run a local Redis instance on `localhost:6379`.

Notes:

- The local Docker setup connects the client directly to the game server WebSocket.
- The `ws-router` is mainly needed for multi-pod or production-style routing.
- The codebase targets Go `1.25` and Node.js `20` in CI.

## Common Commands

```bash
make build
make test
make lint
make typecheck
```

These commands cover:

- frontend build, lint, tests, and TypeScript checks
- Go build, lint, and test runs for `server`, `api`, and `ws-router`

## Deployment And Ops

The repo includes Kubernetes and observability assets for running AstroDrift outside local Compose:

- `k8s/base`: shared manifests for the web client, API server, game server, Redis, and ingress
- `k8s/overlays/dev`: development overlay
- `k8s/overlays/prod`: production overlay
- `k8s/components/monitoring`: ServiceMonitor and PrometheusRule resources
- `k8s/helm`: Helm values for Prometheus, Loki, and Promtail
- `k8s/argocd`: Argo CD application and image updater manifests

Helpful targets:

```bash
make k8s-dev
make k8s-prod
make k8s-dev-monitoring
make k8s-prod-monitoring
make k8s-argocd
make k3d-up
make load-images-dev
make k8s-apply-dev
```

## CI

GitHub Actions validates the repo by:

- linting and testing all Go modules
- linting, testing, and building the client
- rendering Kubernetes overlays
- building Docker images for each service

## Project Layout

```text
.
├── api
├── client
├── config
├── k8s
├── server
└── ws-router
```

## Development Notes

- The frontend title and in-game branding are `AstroDrift`.
- Local matchmaking currently exposes a `Local` region in the menu.
- The API signs player tokens and can return either a direct game-server URL or a `ws-router` URL, depending on configuration.
- Leaderboard writes are signed before the game server reports results to the API.
