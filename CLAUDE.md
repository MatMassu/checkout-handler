# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Run the server
go run ./cmd/api

# Build
go build -o api ./cmd/api

# Run with Docker
docker compose up --build

# Run tests
go test ./...

# Run a single test
go test ./internal/orders/... -run TestName

# Tidy dependencies
go mod tidy
```

## Environment

Copy `.env` and populate with your own values:
```
DATABASE_URL=postgresql://...
PORT=8080
```

The app loads `.env` automatically via `godotenv` on startup.

## Architecture

This is a backend service for the Vinilo Market e-commerce platform. It uses **standard `net/http`** (no framework), **pgx/v5** for PostgreSQL (Neon serverless), and follows a layered handler → service pattern.

**Entry point:** `cmd/api/main.go` — wires together the DB pool, services, and handlers, then registers routes directly on `http.DefaultServeMux`.

**Package layout under `internal/`:**
- `database/` — creates a `pgxpool.Pool` from `DATABASE_URL`; pool config is fixed (max 10, min 2 conns)
- `orders/` — the only fully implemented domain: `model.go` (request/response types), `service.go` (DB writes in a transaction), `handler.go` (HTTP decode → service → JSON response)
- `payments/` — planned MercadoPago integration (handler, service, client, webhook stubs — currently empty)
- `stock/` — planned stock validation/reservation (service + repository stubs — currently empty)
- `config/`, `middleware/`, `router/`, `app/` — stub packages not yet implemented

**Request flow (orders):**
```
POST /checkout
  → orders.Handler.Checkout      (decode + validate)
  → orders.Service.CreateOrder   (transaction: INSERT orders + INSERT order_items)
  → returns order_id + "created" status
```

**Database transactions:** `Service.CreateOrder` uses `tx.Rollback` deferred + explicit `tx.Commit`. All DB operations use `context` from the request.

**Planned but not yet implemented:** stock reservation before order creation, MercadoPago payment session creation, webhook processing for payment status updates, middleware (logging, recovery, CORS), and a proper router abstraction.
