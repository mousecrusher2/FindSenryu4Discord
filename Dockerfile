# Build stage
FROM golang:1.27-alpine@sha256:eb61c3873c97416fff0116553fff65d67a3549f72e11d9758849a689cdd8b523 AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o findsenryu .

# Runtime stage
FROM gcr.io/distroless/static-debian13:nonroot@sha256:aafe993665bf6200db38a02c11a9351e782a84b6a45e7f33b6695638c88e3931

WORKDIR /app
COPY --from=builder /build/findsenryu /app/findsenryu
ENV GODEBUG=disablethp=1

ENTRYPOINT ["/app/findsenryu"]
CMD ["bot"]
