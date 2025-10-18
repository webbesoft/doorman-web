FROM golang:1.24-alpine AS builder

LABEL org.opencontainers.image.source=https://github.com/webbesoft/doorman-go
LABEL org.opencontainers.image.authors="Tawanda Munongo <ejmunongo@gmail.com>"

RUN apk add --no-cache make build-base

RUN go install github.com/a-h/templ/cmd/templ@latest

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN templ generate

RUN CGO_ENABLED=1 GOOS=linux go build -o doorman cmd/doorman

FROM node AS asset-builder

COPY --from=builder /app/assets/css/input.css ./assets/css/input.css

RUN npm install -g tailwindcss @tailwindcss/cli

RUN tailwindcss -i ./assets/css/input.css -o ./assets/css/output.css

FROM alpine:latest AS final
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /root/

COPY --from=builder /app/assets /app/assets
COPY --from=asset-builder /app/assets/css/output.css /app/assets/css/output.css
COPY --from=builder /app/doorman .

EXPOSE 8080

CMD ["./doorman"]