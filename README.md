# MPG Pixelverse Backend

A Docker-based backend written primarily in Go for the <a href="https://github.com/MPixelG/mpg_achievements_app">MPG Pixelverse Game</a>. The backend provides authentication, real-world QR code challenges, and multiplayer game session management.

> **Note:** Many features are not yet fully implemented in the frontend and remain untested. Two test scripts are included for backend testing.

## Quick Start

The entire backend stack can be launched with a single command:

```bash
docker compose up -d
```

All containers will be created and configured automatically.

## Architecture

### Monitoring Stack

**Prometheus & Grafana** are used for metrics collection and visualization. An extensive server overview dashboard is included by default, with additional custom metrics available in Prometheus for creating your own dashboards.

- Grafana UI: `http://localhost:3000`

### Reverse Proxy

**Nginx** serves as the reverse proxy, handling static content delivery and HTTPS encryption.

### Database

**ScyllaDB** (Cassandra-like) stores account data, game saves, and other persistent information.

### Application Server

The Go backend exposes two ports:
- **TCP 9000** - Primary application port
- **UDP 9001** - Game communication port

## Core Features

### Authentication

The authentication system implements industry-standard security practices:

- Passwords are hashed using **Argon2id** with both salt and pepper
- Performance: ~150 logins/registrations per second (limited by Argon2id's intentional computational cost)
- **Session management:**
  - Short-lived jwt access tokens for API requests
  - Long-lived refresh tokens stored in the database
  - When the access token expires a new token pair can be requested

### QR Code System

Designed for real-world challenge integration where players physically locate and scan QR codes:

- QR codes and their associated actions are stored separately
- Each QR code ID references a database entry containing scan rules (e.g., scan limits, cooldowns)
- Action payloads are configurable (implementation pending)

### Game Sessions

The backend follows a client-authoritative architecture, acting primarily as a relay:

- **Rooms** contain multiple **sessions**
- Each **session** represents one authenticated account
- Sessions can spawn **entities** with full client-side control
- Other sessions cannot modify entities they don't own

This design minimizes server-side validation overhead while maintaining session isolation.

## Protocol Specification

All messages follow this binary structure:

| Field | Size | Description |
|-------|------|-------------|
| Magic Bytes | 3 bytes | Protocol identifier: `"MPG"` |
| Message Type | 1 byte | Message command |
| Message Length | 4 bytes | Payload size in bytes |

For a complete list of message types, see `messages.go`.

## Testing

### Performance Testing

1. Start the backend: `docker compose up -d`
2. Navigate to `tests/perf/` and configure parameters in `main.go`
3. Run the test: `go run .`
4. Monitor results in the Grafana dashboard

**Current Performance:** <sub>i5-11400k</sub>
- RX: ~40k packets/second
- TX: ~300k packets/second
- Bottleneck: UDP packet transmission (~1ms, improved from 20+ms using `sendmmsg` syscall)
- Future optimization: DPDK for kernel bypass

### Integration Testing

Tests authentication and QR code functionality (partially deprecated).

1. Create an admin account via `cqlsh` in the ScyllaDB container
2. Run: `python test_auth.py`
