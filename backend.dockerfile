FROM golang:1.25.7-alpine3.23 AS builder

RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o server ./cmd/server/main.go

FROM alpine:3.23
RUN addgroup -S appgroup && adduser -S appuser -G appgroup \
    && apk add --no-cache wget
WORKDIR /app
COPY --from=builder /app/server .
USER appuser
EXPOSE 8080
ENTRYPOINT ["./server"]
