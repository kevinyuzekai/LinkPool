.PHONY: build test run tidy clean build-darwin-arm64 build-darwin-amd64 package-dmg package-dmg-amd64 app

VERSION ?= 0.2.0
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

# Cross-compile for Apple Silicon (works on Linux CI / this box)
build-darwin-arm64:
	ARCH=arm64 ./scripts/build-macos.sh

# Cross-compile for Intel Mac (darwin/amd64)
build-darwin-amd64:
	ARCH=amd64 ./scripts/build-macos.sh

# macOS only — creates .dmg via hdiutil (default arm64)
package-dmg:
	ARCH=arm64 ./scripts/package-dmg.sh

package-dmg-amd64:
	ARCH=amd64 ./scripts/package-dmg.sh

app: build-darwin-arm64
