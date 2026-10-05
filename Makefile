PREFIX ?= $(HOME)/.local
BUILD_DIR ?= build

.PHONY: all build test clean install

all: build

build:
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/updates-fetch ./cmd/updates-fetch
	go build -o $(BUILD_DIR)/updates-query ./cmd/updates-query

test:
	go test -v ./...

clean:
	rm -rf $(BUILD_DIR)

install: build
	install -d $(PREFIX)/bin
	install -m 755 $(BUILD_DIR)/updates-fetch $(PREFIX)/bin/updates-fetch
	install -m 755 $(BUILD_DIR)/updates-query $(PREFIX)/bin/updates-query
