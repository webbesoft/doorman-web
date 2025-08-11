FROM golang:1.24-alpine AS builder

LABEL org.opencontainers.image.source=https://github.com/webbesoft/doorman

RUN apk add --no-cache make build-base

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN templ generate
RUN CGO_ENABLED=1 GOOS=linux go build -o main .

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /root/

COPY --from=builder /app/main .

EXPOSE 8080

CMD ["./main"]