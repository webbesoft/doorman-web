# Match generated code to the templ runtime declared in go.mod.
TEMPL_VERSION := $(shell go list -m -f '{{.Version}}' github.com/a-h/templ)

.PHONY: develop generate build format release test pre_commit

develop:
	APP_ENV=local air

generate:
	go run github.com/a-h/templ/cmd/templ@$(TEMPL_VERSION) generate

# The development executable must match the machine running Air.
build: generate
	GOOS=$(shell go env GOHOSTOS) GOARCH=$(shell go env GOHOSTARCH) go build -o ./tmp/main ./cmd/doorman

format:
	go fmt ./...

release:
	docker build -t ghcr.io/webbesoft/doorman .
	docker push ghcr.io/webbesoft/doorman

test:
	go test ./...

pre_commit: format test
