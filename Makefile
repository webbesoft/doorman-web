.PHONY: develop generate build format release test check pre_commit

develop:
	APP_ENV=local air

generate:
	go tool templ generate

# The development executable must match the machine running Air.
build: generate
	GOOS=$(shell go env GOHOSTOS) GOARCH=$(shell go env GOHOSTARCH) go build -o ./tmp/main ./cmd/doorman

format:
	go fmt ./...

release:
	docker build -t ghcr.io/webbesoft/doorman .
	docker push ghcr.io/webbesoft/doorman

test: generate
	go test -race -count=1 ./...

check: test
	go vet ./...
	go build -o ./tmp/doorman ./cmd/doorman

pre_commit: format test check
