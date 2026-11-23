# Backend Migration Guide: Rust to Node.js (TypeScript) or Go

This document outlines the recommended directory structure and implementation path to convert `restolink` from Rust into a production-ready **Node.js (TypeScript + Fastify)** or **Go** backend.

---

## Option A: Node.js (TypeScript + Fastify) - *Recommended for Fast Fullstack Velocity*

### Directory Structure:
```
restolink/
├── package.json
├── tsconfig.json
├── src/
│   ├── server.ts                  # App bootstrapping, plugin registration
│   ├── plugins/
│   │   ├── postgres.ts            # Database pool (pg / drizzle / kysely)
│   │   ├── redis.ts               # Redis client & Pub/Sub
│   │   ├── jwt.ts                 # JWT verification & token blacklist
│   │   └── websocket.ts           # Fastify WebSocket handling
│   ├── modules/
│   │   ├── auth/                  # Login, registration, token refresh
│   │   │   ├── auth.routes.ts
│   │   │   ├── auth.service.ts
│   │   │   └── auth.schema.ts
│   │   ├── tenant/                # Tenant resolution, branding config
│   │   ├── menu/                  # Categories, menu items, modifier groups
│   │   ├── orders/                # Order placement, state machine, KDS
│   │   │   ├── orders.routes.ts
│   │   │   ├── orders.service.ts
│   │   │   └── state-machine.ts
│   │   ├── delivery/              # Live GPS tracking, driver assignments
│   │   └── notifications/         # WhatsApp Cloud API & Web Push workers
│   └── shared/
│       ├── config.ts              # Env validation with Zod
│       └── errors.ts              # Structured API errors
```

### Essential Dependencies:
- `fastify`: High performance HTTP/WebSocket framework.
- `@fastify/websocket`: Built-in WebSocket routing.
- `drizzle-orm` + `postgres`: Type-safe SQL builder with zero runtime overhead.
- `zod`: Request and response validation.
- `ioredis`: Robust Redis client supporting Pub/Sub and Geospatial commands.
- `pino`: Lightning fast structured JSON logger.

---

## Option B: Go (Fiber / Chi) - *Recommended for Ultra-Low Memory & Standalone Binaries*

### Directory Structure:
```
restolink/
├── go.mod
├── go.sum
├── cmd/
│   └── api/
│       └── main.go                # Application entrypoint
├── internal/
│   ├── config/                    # Config parser from env
│   ├── database/                  # pgxpool connection
│   ├── redis/                     # go-redis client & pubsub
│   ├── middleware/                # Tenant resolver, JWT auth guard
│   ├── models/                    # Database structs & JSON tags
│   ├── modules/
│   │   ├── auth/                  # Handlers, services, repo
│   │   ├── tenant/
│   │   ├── menu/
│   │   ├── orders/                # State machine & order logic
│   │   ├── delivery/              # Driver GPS ingestion & Redis GEO
│   │   └── notifications/         # WhatsApp worker
│   └── realtime/                  # Gorilla WebSocket / SSE hub
```

### Essential Go Modules:
- `github.com/gofiber/fiber/v2` or `github.com/go-chi/chi/v5`
- `github.com/jackc/pgx/v5` (High-performance PostgreSQL driver)
- `github.com/redis/go-redis/v9` (Redis client with GEO and Pub/Sub support)
- `github.com/golang-jwt/jwt/v5`
