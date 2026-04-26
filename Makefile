.PHONY: build install test vet clean lint

BINARY = pfs
GO = go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS = -ldflags "-s -w -X main.Version=$(VERSION)"

build:
	$(GO) build $(LDFLAGS) -o $(BINARY) ./cmd/

install: build
	cp $(BINARY) /usr/local/bin/$(BINARY)

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

clean:
	rm -f $(BINARY)

lint: vet
