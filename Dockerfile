# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o api ./cmd/api

# Runtime stage
FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/api .

EXPOSE 8080

CMD ["./api"]
