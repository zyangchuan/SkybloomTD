FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /user-service .

FROM alpine:3.20

RUN adduser -D -H appuser

WORKDIR /app

COPY --from=builder /user-service /app/user-service

EXPOSE 8080

USER appuser

CMD ["/app/user-service"]
