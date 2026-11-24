.PHONY: build install test test-short vet clean lint

BINARY = pfs
GO = go
PREFIX ?= /usr/local
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS = -ldflags "-s -w -X main.Version=$(VERSION)"

build:
	$(GO) build $(LDFLAGS) -o $(BINARY) ./cmd/

# Installs the binary and the shell wrappers. `pfs setup` finds the wrappers
# in $(PREFIX)/share/pfs/shell.
install: build
	install -d $(DESTDIR)$(PREFIX)/bin $(DESTDIR)$(PREFIX)/share/pfs/shell
	install -m 0755 $(BINARY) $(DESTDIR)$(PREFIX)/bin/$(BINARY)
	install -m 0644 shell/pfs.bash shell/pfs.zsh shell/pfs.fish $(DESTDIR)$(PREFIX)/share/pfs/shell/

# Full suite. The shell tests drive real bash, zsh and fish in a pseudo-terminal
# (they need python3 and skip shells that are not installed).
test:
	$(GO) test ./...

# Skips the end-to-end terminal test, which builds the binary.
test-short:
	$(GO) test -short ./...

vet:
	$(GO) vet ./...

clean:
	rm -f $(BINARY)

lint: vet
