format:
	go fmt

release:
	docker build -t ghcr.io/webbesoft/doorman .
	docker push ghcr.io/webbesoft/doorman