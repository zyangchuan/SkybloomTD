FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /document-content-grpc .

FROM alpine:3.20

RUN adduser -D -H appuser

WORKDIR /app

COPY --from=builder /document-content-grpc /app/document-content-grpc

EXPOSE 50051

USER appuser

CMD ["/app/document-content-grpc"]
