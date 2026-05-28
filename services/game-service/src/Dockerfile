FROM golang:1.23-alpine AS dev

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

EXPOSE 8081

CMD ["go", "run", "."]

FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /game-service .

FROM alpine:3.20

RUN adduser -D -H appuser

WORKDIR /app

COPY --from=builder /game-service /app/game-service

EXPOSE 8081

USER appuser

CMD ["/app/game-service"]
