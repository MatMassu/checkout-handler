# Checkout Handler 
Backend service for the Vinilo Market e-commerce platform.

Built with:
- Go 1.26
- PostgreSQL (Neon serverless)
- Docker

This service handles:
- Order creation
- Stock validation and reservation
- Payment session creation
- Webhook processing

---

## Architecture

Next.js (Frontend)
        ↓
Go API (this service)
        ↓
Neon PostgreSQL (cloud database)

---

## Requirements

- Go 1.26+
- Docker (optional but recommended)
- Neon PostgreSQL database
