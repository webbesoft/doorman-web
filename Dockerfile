FROM golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS builder
RUN apk add --no-cache make build-base
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN --mount=type=cache,id=doorman-go-build,target=/root/.cache/go-build make generate
RUN --mount=type=cache,id=doorman-go-build,target=/root/.cache/go-build CGO_ENABLED=1 go build -trimpath -o /out/doorman ./cmd/doorman

FROM alpine:3.24@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf AS final
LABEL org.opencontainers.image.source=https://github.com/webbesoft/doorman-web
RUN apk add --no-cache ca-certificates tzdata libgcc && addgroup -S doorman && adduser -S -G doorman doorman && mkdir -p /data && chown doorman:doorman /data
WORKDIR /app
COPY --from=builder /out/doorman /app/doorman
USER doorman
ENV APP_ENV=production DB_PROVIDER=sqlite DB_PATH=/data/analytics.db
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD ["/app/doorman", "-healthcheck"]
ENTRYPOINT ["/app/doorman"]
