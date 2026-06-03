# Build stage
FROM golang:1.26-alpine@sha256:f23e8b227fb4493eabe03bede4d5a32d04092da71962f1fb79b5f7d1e6c2a17f AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o bot main.go
RUN CGO_ENABLED=0 go build -o migrate ./cmd/migrate

# Runtime stage
FROM gcr.io/distroless/base-debian13:nonroot@sha256:a557d784ac275c287d2bdf3172f47bece8d2a0ef3c0fdefb712e95084a04a562

WORKDIR /app
COPY --from=builder /build/bot /app/bot
COPY --from=builder /build/migrate /app/migrate

EXPOSE 9090

ENTRYPOINT ["/app/bot"]
