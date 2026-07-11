default: build

build:
	go build -o terraform-provider-salesforce

lint:
	golangci-lint run

test:
	go test ./... -v

testacc:
	TF_ACC=1 go test ./... -v -timeout 120m

fmt:
	go fmt ./...

tidy:
	go mod tidy

.PHONY: default build lint test testacc fmt tidy
