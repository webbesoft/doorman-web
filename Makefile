develop:
	APP_ENV=local air

format:
	go fmt ./...

release:
	docker build -t ghcr.io/webbesoft/doorman .
	docker push ghcr.io/webbesoft/doorman

tw_watch:
	tailwindcss -i ./assets/css/input.css -o ./assets/css/output.css --watch

test:
	go test ./...

pre_commit: format test
