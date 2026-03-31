# -------- STAGE 1: Build --------
FROM golang:1.25.7-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -o cache-proxy ./cmd/server


# -------- STAGE 2: Run --------
FROM alpine:3.20

RUN apk add --no-cache ca-certificates && update-ca-certificates

WORKDIR /app

COPY --from=builder /app/cache-proxy .

EXPOSE 8080

CMD ["./cache-proxy"]
