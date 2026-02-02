FROM golang:1.23-bookworm AS builder
WORKDIR /app
COPY backend/ .
RUN go mod download
RUN go build -o main .

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y \
    chromium \
    chromium-driver \
    ca-certificates \
    dumb-init \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /app/main .
RUN mkdir -p /app/data
ENTRYPOINT ["/usr/bin/dumb-init", "--"]
CMD ["./main"]