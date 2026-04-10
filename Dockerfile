FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /agent-memory ./cmd/server

FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /agent-memory .
COPY config.yaml .

EXPOSE 8100

ENTRYPOINT ["/app/agent-memory"]
CMD ["-config", "/app/config.yaml"]
