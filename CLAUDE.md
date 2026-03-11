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
go test ./internal/checkout/... -run TestName

# Tidy dependencies
go mod tidy
```

## Environment

Copy `.env` and populate with your own values:
```
DATABASE_URL=postgresql://...
PORT=8080
MERCADOPAGO_ACCESS_TOKEN=...
MERCADOPAGO_WEBHOOK_SECRET=...
MERCADOPAGO_NOTIFICATION_URL=https://<ngrok-or-prod-host>/payments/webhook
MERCADOPAGO_SANDBOX=true
ALLOWED_ORIGIN=https://vinilomarket.vercel.app
```

The app loads `.env` automatically via `godotenv` on startup.

## Architecture

Backend service for the Vinilo Market e-commerce platform. Uses **standard `net/http`** (no framework), **pgx/v5** for PostgreSQL (Neon serverless), and follows a **DDD-inspired layered architecture**.

**Entry point:** `cmd/api/main.go` — loads `.env`, delegates entirely to `server.Start()`.

**Package layout under `internal/`:**
- `domain/` — pure types and constants: `Order`, `Product`, `MPPayment`, status constants (`pending`, `paid`, `failed`, `cancelled`, `expired`), sentinel errors (`ErrProductNotFound`, `ErrInsufficientStock`). No dependencies on other internal packages.
- `checkout/` — checkout feature: `Service` (order creation, payment confirmation), `Controller` (HTTP), `ExpiryWorker` (background ticker), `interfaces.go` (defines `Repository` and `PaymentStarter`), `dto/` (request/response types).
- `payment/` — payment feature: `Service` (preference creation, webhook processing), `Controller` (HTTP), `signature.go` (HMAC-SHA256 validation), `interfaces.go` (defines `OrderConfirmer`, `DBRepository`, `MPRepository`), `dto/` (webhook notification type).
- `repository/` — infrastructure implementations: `postgres.go` (implements `checkout.Repository` + `payment.DBRepository`), `mercadopago.go` (implements `payment.MPRepository`, makes HTTP calls to MercadoPago API).
- `middleware/` — stubs for `cors.go`, `logging.go`, `recovery.go` (not yet implemented).
- `server/` — wiring: `server.go` (config loading, graceful shutdown), `routes.go` (route registration), `dependencies.go` (constructs and wires all services).
- `database/` — creates a `pgxpool.Pool` from `DATABASE_URL`; pool config is fixed (max 10, min 2 conns).

**Dependency direction:** `domain` ← `repository` ← `server`; `checkout` and `payment` are peers that never import each other. The circular init (`checkout.Service` needs `payment.Service` as `PaymentStarter`, `payment.Service` needs `checkout.Service` as `OrderConfirmer`) is broken by `checkout.Service.SetPayments()`, called in `server/dependencies.go` after both services are created.

**Request flow:**
```
POST /checkout
  → checkout.Controller.Checkout    (decode + validate)
  → checkout.Service.Checkout       (consolidate quantities, create order)
  → repository.Postgres.CreateOrder (transaction: lock products FOR UPDATE in UUID order,
                                     validate stock, INSERT order + items, UPDATE reserved)
  → payment.Service.StartPayment    (create MP preference, INSERT payment record)
  → returns order_id, status, expires_at, payment_url

POST /payments/webhook
  → payment.Controller.Webhook      (decode notification)
  → payment.Service.ProcessWebhook  (validate HMAC-SHA256 signature, fetch payment from MP API)
  → repository.Postgres.UpdatePayment
  → checkout.Service.ConfirmPayment → repository.Postgres.ConfirmPayment
                                      (lock order FOR UPDATE, update stock + order status)
```

**Key invariants:**
- Stock locking: product rows locked in sorted UUID order inside every transaction to prevent deadlocks.
- Status transitions: order row re-locked with `SELECT ... FOR UPDATE` before any status change; silently no-ops if already transitioned (guards against expiry worker vs. webhook races and duplicate deliveries).
- Stock release: always uses `GREATEST(reserved - qty, 0)` to prevent negative values.
- Prices stored as whole ARS pesos (`BIGINT`), not centavos.
- `expires_at` set via `NOW() + interval '10 minutes'` on the DB clock, returned via `RETURNING`.

**Planned but not yet implemented:** Railway deployment.
