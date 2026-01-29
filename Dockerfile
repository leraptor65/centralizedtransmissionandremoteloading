# Build Go Backend
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -o server .

# Final Image
FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/server .

# Expose port
EXPOSE 1337

CMD ["./server"]
