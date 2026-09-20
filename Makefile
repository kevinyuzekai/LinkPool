.PHONY: build test run tidy clean build-darwin-arm64 package-dmg app

VERSION ?= 0.1.0
LDFLAGS := -s -w -X main.version=$(VERSION)

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/linkpool ./cmd/linkpool

test:
	go test ./...

run: build
	./bin/linkpool -open

tidy:
	go mod tidy

clean:
	rm -rf bin build

# Cross-compile CLI/core for Apple Silicon (works on Linux CI / this box)
build-darwin-arm64:
	./scripts/build-macos-arm64.sh

# macOS only — creates .dmg via hdiutil
package-dmg:
	./scripts/package-dmg.sh

app: build-darwin-arm64
