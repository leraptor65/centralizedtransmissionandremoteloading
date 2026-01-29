# Stage 1: Build React Frontend
FROM node:lts-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm install
COPY frontend/ .
RUN npm run build

# Stage 2: Build Go Backend
FROM golang:1.25-alpine AS backend-builder
WORKDIR /app/backend
COPY backend/go.* ./
RUN go mod download
COPY backend/ .
# Copy built assets from frontend stage
COPY --from=frontend-builder /app/frontend/dist ./frontend/dist
# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -o server .

# Stage 3: Final Image
FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=backend-builder /app/backend/server .

# Expose port
EXPOSE 1337

# Volume for config
VOLUME ["/root/config_mount"]

CMD ["./server"]
