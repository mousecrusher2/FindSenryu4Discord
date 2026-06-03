# Build stage
FROM golang:1.26-alpine@sha256:f23e8b227fb4493eabe03bede4d5a32d04092da71962f1fb79b5f7d1e6c2a17f AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o bot main.go
RUN CGO_ENABLED=0 go build -o migrate ./cmd/migrate

# Runtime stage
FROM gcr.io/distroless/static-debian13:nonroot@sha256:963fa6c544fe5ce420f1f54fb88b6fb01479f054c8056d0f74cc2c6000df5240

WORKDIR /app
COPY --from=builder /build/bot /app/bot
COPY --from=builder /build/migrate /app/migrate

ENTRYPOINT ["/app/bot"]
