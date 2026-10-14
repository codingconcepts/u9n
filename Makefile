test:
	go test ./... -v

lint:
	go vet ./...
	golangci-lint run --fix --enable-only wsl_v5 --tests
	golangci-lint run --fix --enable-only modernize --tests