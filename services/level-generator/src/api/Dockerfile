FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /level-generator-api .

FROM alpine:3.20

RUN adduser -D -H appuser

WORKDIR /app

COPY --from=builder /level-generator-api /app/level-generator-api

EXPOSE 8000

USER appuser

CMD ["/app/level-generator-api"]
