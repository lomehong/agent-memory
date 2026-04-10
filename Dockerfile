FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
COPY frontend/ /tmp/frontend/
RUN mkdir -p cmd/server/web && cp -r /tmp/frontend/* cmd/server/web/
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /agent-memory ./cmd/server/

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /agent-memory .
COPY config.yaml .
EXPOSE 8101
CMD ["./agent-memory"]
