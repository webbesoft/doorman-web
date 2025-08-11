develop:
	APP_ENV=local air

format:
	go fmt

release:
	docker build -t ghcr.io/webbesoft/doorman .
	docker push ghcr.io/webbesoft/doorman

tw-watch:
	tailwindcss -i ./assets/css/input.css -o ./assets/css/output.css --watch