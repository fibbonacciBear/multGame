## Game Server Config Files

- `game-server.env` is the shared, committed baseline used by local compose.
- `game-server.local.env` is for local-only overrides and is gitignored.
- `game-server.local.env.example` is a template for common overrides.

Compose loads both files for `game-server`, with local overrides taking precedence.
