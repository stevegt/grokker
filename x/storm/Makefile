.PHONY: build clean test

# Default target
all: 
	@echo "Some available targets:"
	@echo "  test         - Run tests"
	@echo "  dev          - Build and run './storm serve' for touch testing with live db"
	@echo "  prod         - Install storm in the Go bin directory and run 'storm serve'"

# Get version from git: use tag if current commit is tagged, otherwise use short commit hash
VERSION := $(shell git describe --tags --exact-match 2>/dev/null || echo "dev-$$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown')")

ldflags := -X github.com/stevegt/grokker/x/storm/version.Version=$(VERSION)

# Build the storm binary with version injection via ldflags and save
# it in ./bin/storm-$(VERSION)
build: 
	@echo "Building storm version: $(VERSION)"
	mkdir -p bin
	go build -ldflags="$(ldflags)" -o bin/storm-$(VERSION) 

# build the binary, symlink to ./storm, and run serve 
dev: vet commit-ck build symlink
	- storm stop
	- ./storm stop
	./storm serve 

vet:
	go vet ./...

symlink:
	ln -sf bin/storm-$(VERSION) storm

# Install the binary in the Go bin directory
install: 
	go install -v -ldflags="$(ldflags)" 

# Run tests
test:
	go test -v -p 1 -failfast ./...

# Clean build artifacts
clean:
	rm -f storm
	go clean

# Show version that will be built
version:
	@echo "Next build version: $(VERSION)"

# install and run serve command 
prod: version-check 
	$(MAKE) build symlink install
	- storm stop
	storm serve 

# Run the serve command with development database
test-serve: 
	@echo remember to set STORM_DAEMON_URL=http://localhost:9090
	test -n "$(STORM_DAEMON_URL)"
	go run . serve -d /tmp/storm-test.db -p 9090

commit-ck:
	git diff --quiet --exit-code || (echo "Working directory is dirty. Please commit changes." && exit 1)

# ensure the version number in core/grokker.go is newer than the latest git tag
version-check:
	./version-check.sh

.PHONY: all build install test clean version dev-build test-serve
