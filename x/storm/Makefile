.PHONY: build clean test

# Default target
all: dev

# Get version from git: use tag if current commit is tagged, otherwise use short commit hash
VERSION := $(shell git describe --tags --exact-match 2>/dev/null || echo "dev-$$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown')")

ldflags := -X github.com/stevegt/grokker/x/storm/version.Version=$(VERSION)

# Build the storm binary with version injection via ldflags
build: commit-ck
	@echo "Building storm version: $(VERSION)"
	go build -ldflags="$(ldflags)" -o storm

# build the binary for development use and run the serve command
dev: build
	- storm stop
	- ./storm stop
	./storm serve 

# Install the binary in the Go bin directory
install: commit-ck
	go install -v -ldflags="$(ldflags)" 

# Run tests
test:
	go test -v -p 1 ./...

# Clean build artifacts
clean:
	rm -f storm
	go clean

# Show version that will be built
version:
	@echo "Next build version: $(VERSION)"

# install and run serve command 
serve: install
	- storm stop
	storm serve 

# Run the serve command with development database
test-serve: 
	@echo remember to set STORM_DAEMON_URL=http://localhost:9090
	test -n "$(STORM_DAEMON_URL)"
	go run . serve -d /tmp/storm-test.db -p 9090

commit-ck:
	git diff --quiet --exit-code || (echo "Uncommitted changes found.")

.PHONY: all build install test clean version dev-build test-serve
