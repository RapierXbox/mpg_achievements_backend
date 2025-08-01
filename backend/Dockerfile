# build stage
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git make

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# build the go server
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/bin/server ./cmd/server

# runtime stage
FROM alpine:3.18

# install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata wget

# create app user
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

# copy built binary and entrypoint
COPY --from=builder /app/bin/server .

RUN chown -R appuser:appgroup /app

USER appuser

EXPOSE 8080

# health check endpoint
HEALTHCHECK --interval=30s --timeout=3s \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

CMD ["./server"]
