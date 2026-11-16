FROM golang:1.27.4-alpine AS builder
RUN apk add --no-cache make build-base
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make generate
RUN CGO_ENABLED=1 go build -trimpath -o /out/doorman ./cmd/doorman

FROM alpine:3.24 AS final
LABEL org.opencontainers.image.source=https://github.com/webbesoft/doorman-web
RUN apk add --no-cache ca-certificates tzdata libgcc && addgroup -S doorman && adduser -S -G doorman doorman && mkdir -p /data && chown doorman:doorman /data
WORKDIR /app
COPY --from=builder /out/doorman /app/doorman
USER doorman
ENV APP_ENV=production DB_PROVIDER=sqlite DB_PATH=/data/analytics.db
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD ["/app/doorman", "-healthcheck"]
ENTRYPOINT ["/app/doorman"]
