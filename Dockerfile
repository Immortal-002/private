# Use Debian-based images (avoids Alpine CDN issues in some regions)
FROM golang:1.23-bookworm AS builder

WORKDIR /app

COPY go.mod go.sum* ./
COPY vendor/ vendor/

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -ldflags="-w -s" -o /telemetry-server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=builder /telemetry-server .

EXPOSE 8080

CMD ["./telemetry-server"]
