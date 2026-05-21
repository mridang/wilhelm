.PHONY: default build test lint format format-check generate generate-all clean clean-coverage ensure-gotestsum crd-list verify-all release-dry

export GO111MODULE=on

GOTESTSUM ?= gotestsum
TEST_FORMAT ?= pkgname-and-test-fails
COVER_DIR := .out/cover
JUNIT_FILE := .out/junit.xml
LCOV_FILE := .out/lcov.info

default: test

build:
	go build ./...

test: clean-coverage ensure-gotestsum
	mkdir -p $(dir $(LCOV_FILE))
	$(GOTESTSUM) --junitfile $(JUNIT_FILE) --format $(TEST_FORMAT) -- -coverpkg=./... -covermode=atomic -coverprofile=$(LCOV_FILE) -count=1 ./...

lint:
	golangci-lint run --verbose

format:
	dprint fmt
	go fmt ./...

format-check:
	dprint check
	test -z "$$(gofmt -l .)"

generate:
	go generate ./...

# Run `go generate` in every wilhelm submodule (root + each CRD pkg).
generate-all: generate
	@for mod in $$(find assert env -name go.mod 2>/dev/null); do \
		dir=$$(dirname $$mod); \
		echo "=> $$dir"; \
		(cd $$dir && go generate ./...) || exit 1; \
	done

# crd-list prints every wilhelm submodule directory (for CI verification).
crd-list:
	@find assert env -name go.mod 2>/dev/null | xargs -n1 dirname | sort

# verify-all builds and lints every wilhelm module (root + every submodule).
verify-all:
	@bash hack/verify-all.sh

# release-dry runs hack/release.sh with a fake version against a copy of
# every go.mod, then diffs the result. Use to sanity-check the release
# rewrite before semantic-release invokes it for real.
release-dry:
	@tmp=$$(mktemp -d) && trap "rm -rf $$tmp" EXIT && \
		cp -r go.mod go.sum go.work assert env $$tmp/ && \
		(cd $$tmp && bash $(CURDIR)/hack/release.sh 0.0.0-dry) && \
		echo "=== diff ===" && \
		diff -ruN go.mod $$tmp/go.mod || true && \
		for m in $$(find $$tmp/assert $$tmp/env -name go.mod); do \
			rel=$${m#$$tmp/}; \
			diff -ruN $$rel $$m || true; \
		done

clean-coverage:
	rm -rf .out

clean: clean-coverage
	rm -rf build

ensure-gotestsum:
	@if ! command -v $(GOTESTSUM) >/dev/null 2>&1; then \
		GOFLAGS=-mod=mod go install gotest.tools/gotestsum@latest; \
	fi
