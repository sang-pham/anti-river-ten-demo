# Multi-stage build for go-demo API
# Build stage
FROM golang:1.24-alpine AS build
WORKDIR /src

# Enable Go modules and caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# Runtime stage
FROM alpine:3.20
RUN adduser -D -g '' appuser \
  && apk add --no-cache ca-certificates
WORKDIR /app

# Copy binary
COPY --from=build /out/api /app/api

USER appuser

EXPOSE 8080
ENTRYPOINT ["/app/api"]