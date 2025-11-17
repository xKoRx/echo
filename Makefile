.PHONY: help proto build test lint clean install-tools

GOLANGCI ?= $(shell go env GOPATH)/bin/golangci-lint

help: ## Muestra esta ayuda
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

install-tools: ## Instala herramientas de desarrollo
	@echo "Instalando buf..."
	go install github.com/bufbuild/buf/cmd/buf@latest
	@echo "Instalando protoc-gen-go..."
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@echo "Instalando protoc-gen-go-grpc..."
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@echo "Instalando golangci-lint..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "Instalando mockery..."
	go install github.com/vektra/mockery/v2@latest

proto: ## Genera código desde proto files
	@echo "Generando código protobuf..."
	cd sdk/proto && buf generate
	@echo "Protobuf generado exitosamente"

build-core: ## Compila el core
	@echo "Compilando echo-core..."
	cd core && go build -o ../bin/echo-core ./cmd/echo-core
	@echo "Core compilado en bin/echo-core"

build-agent: ## Compila el agent
	@echo "Compilando echo-agent..."
	cd agent && go build -o ../bin/echo-agent ./cmd/echo-agent
	@echo "Agent compilado en bin/echo-agent"

build: build-core build-agent ## Compila todos los binarios

test: ## Ejecuta tests de todos los módulos
	@echo "Ejecutando tests..."
	cd core && go test -v -race -coverprofile=coverage.out ./...
	cd agent && go test -v -race -coverprofile=coverage.out ./...
	cd sdk && go test -v -race -coverprofile=coverage.out ./...

test-e2e: ## Ejecuta tests end-to-end
	@echo "Ejecutando tests E2E..."
	cd test_e2e && go test -v -timeout 5m ./...

lint: ## Ejecuta linters
	@echo "Ejecutando linters..."
	cd core && $(GOLANGCI) run ./cmd/... ./internal/...
	cd agent && $(GOLANGCI) run ./internal/... ./cmd/...
	cd sdk && $(GOLANGCI) run ./telemetry/... ./telemetry/metricbundle/... ./telemetry/semconv/...

fmt: ## Formatea código
	@echo "Formateando código..."
	gofmt -s -w .

tidy: ## Limpia dependencias
	@echo "Limpiando dependencias..."
	cd core && go mod tidy
	cd agent && go mod tidy
	cd sdk && go mod tidy
	cd test_e2e && go mod tidy

clean: ## Limpia binarios y archivos generados
	@echo "Limpiando..."
	rm -rf bin/
	rm -rf dist/
	find . -name "coverage.out" -delete
	find . -name "*.pb.go" -delete

mocks: ## Genera mocks para testing
	@echo "Generando mocks..."
	cd sdk && mockery --all --output=mocks --case=underscore

.DEFAULT_GOAL := help

