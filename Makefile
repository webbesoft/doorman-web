.PHONY: develop generate build format release test check security container-check backend-check pre_commit

develop:
	APP_ENV=local air

generate:
	go tool templ generate

# The development executable must match the machine running Air.
build: generate
	GOOS=$(shell go env GOHOSTOS) GOARCH=$(shell go env GOHOSTARCH) go build -o ./tmp/main ./cmd/doorman

format:
	go fmt ./...

release: check security
	@set -eu; \
	test -n "$(VERSION)" || (echo "Set VERSION to an immutable release tag" && exit 1); \
	image_dir=$$(mktemp -d); trap 'rm -rf "$$image_dir"' EXIT; \
	docker build --iidfile "$$image_dir/id" -t doorman-check:local .; \
	image=$$(cat "$$image_dir/id"); \
	python3 tests/container_smoke.py "$$image"; \
	docker tag "$$image" ghcr.io/webbesoft/doorman-web:$(VERSION); \
	docker push ghcr.io/webbesoft/doorman-web:$(VERSION)

test: generate
	go test -race -count=1 ./...
	node --test tests/*.test.cjs
	python3 -m unittest discover -s tests -p '*_test.py'

check: test
	go vet ./...
	go build -o ./tmp/doorman ./cmd/doorman

pre_commit: format test check

security: generate
	mkdir -p tmp
	python3 scripts/check-vulnerabilities.py tmp/vulnerabilities.json

container-check:
	@set -eu; \
	image_dir=$$(mktemp -d); trap 'rm -rf "$$image_dir"' EXIT; \
	docker build --iidfile "$$image_dir/id" -t doorman-check:local .; \
	python3 tests/container_smoke.py "$$(cat "$$image_dir/id")"

backend-check:
	python3 tests/run_backend_tests.py
