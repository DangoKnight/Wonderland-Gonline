GO ?= go
GOLANGCI_LINT ?= $(shell which golangci-lint 2>/dev/null || which $(shell $(GO) env GOPATH 2>/dev/null)/bin/golangci-lint 2>/dev/null || echo "")

.PHONY: build test race vet inspect run fmt fmt-check lint lint-go lint-html
build:
	$(GO) build -o bin/wonderland ./cmd/wonderland

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

inspect:
	$(GO) run ./cmd/wonderland -inspect-data

run:
	$(GO) run ./cmd/wonderland -config config.example.json

fmt:
	gofmt -s -w .
	@if command -v prettier >/dev/null 2>&1; then \
		prettier --write "internal/admin/web/**/*.{html,css,js}"; \
	elif command -v npx >/dev/null 2>&1; then \
		npx prettier --write "internal/admin/web/**/*.{html,css,js}"; \
	fi

fmt-check:
	@DIFF=$$(gofmt -l .); if [ -n "$$DIFF" ]; then echo "Unformatted Go files:"; echo "$$DIFF"; exit 1; fi
	@if command -v prettier >/dev/null 2>&1; then \
		prettier --check "internal/admin/web/**/*.{html,css,js}"; \
	elif command -v npx >/dev/null 2>&1; then \
		npx prettier --check "internal/admin/web/**/*.{html,css,js}"; \
	fi

lint: lint-go lint-html

lint-go: vet
	@if [ -n "$(GOLANGCI_LINT)" ]; then \
		$(GOLANGCI_LINT) run ./...; \
	fi

lint-html:
	@if command -v htmlhint >/dev/null 2>&1; then \
		htmlhint --config .htmlhintrc internal/admin/web/index.html; \
	elif command -v npx >/dev/null 2>&1; then \
		npx htmlhint --config .htmlhintrc internal/admin/web/index.html; \
	else \
		echo "htmlhint not found. Please install node/npm to lint HTML." && exit 1; \
	fi

