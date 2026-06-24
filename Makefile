VERSION ?= 0.1.0
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
PREFIX  ?= /usr/local

.PHONY: build install test vet clean

build:
	go build $(LDFLAGS) -o hc .

install: build
	install -m 0755 hc $(PREFIX)/bin/hc

vet:
	go vet ./...

test: vet
	go test ./...

clean:
	rm -f hc
