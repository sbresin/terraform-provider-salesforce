GOLANGCI_LINT ?= go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
GOVULNCHECK   ?= go run golang.org/x/vuln/cmd/govulncheck@v1.9.0
TFPLUGINDOCS  ?= go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.25.0

default: build

build:
	go build -o terraform-provider-salesforce

lint:
	$(GOLANGCI_LINT) run

vuln:
	$(GOVULNCHECK) ./...

test:
	go test ./... -v

testacc:
	TF_ACC=1 go test ./... -v -timeout 120m

generate:
	$(TFPLUGINDOCS) generate --provider-name salesforce

fmt:
	gofmt -s -w .

tidy:
	go mod tidy

.PHONY: default build lint vuln test testacc generate fmt tidy
