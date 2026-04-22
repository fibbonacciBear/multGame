## Game Server Config Files

- `game-server.env` is the shared, committed baseline used by local compose.
- `game-server.local.env` is for local-only overrides and is gitignored.
- `game-server.local.env.example` is a template for common overrides.

Compose loads both files for `game-server`, with local overrides taking precedence.

## Local Match Analytics Postgres

Compose includes a local `postgres` service for match analytics development. The
API and game server keep `MATCH_ANALYTICS_ENABLED=false` by default so the stack
can run before migrations are applied.

To initialize or update the local schema:

```sh
docker compose --profile tools run --rm api-migrate
```

After migrations are applied, set `MATCH_ANALYTICS_ENABLED=true` for both
`api-server` and `game-server` in your local compose overrides when you want to
exercise reporting end to end.
