# my-week — build & install
#
# Override PREFIX to install elsewhere:
#   make install PREFIX=/some/dir
#
# PREFIX defaults to $HOME/bin.

PREFIX ?= $(HOME)/bin
BIN     = mw

.PHONY: build install test fmt vet clean

build:
	go build -o $(BIN) ./cmd/mw

install: build
	@mkdir -p $(PREFIX)
	install -m 0755 $(BIN) $(PREFIX)/$(BIN)
	@echo "Installed $(PREFIX)/$(BIN)"

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

clean:
	rm -f $(BIN)
