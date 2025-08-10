FROM golang:1.24-alpine AS builder

LABEL org.opencontainers.image.source=https://github.com/webbesoft/doorman

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o main .

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /root/

COPY --from=builder /app/main .

EXPOSE 8080

CMD ["./main"]