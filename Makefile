# VayuMail-Mobile build automation.
# Pure Go, no cgo, one binary per platform.

GO        ?= go
GOFLAGS   ?=
MODULE    := github.com/johalputt/VayuMail-Mobile
BIN_DIR   := dist

.PHONY: all build cli test race lint fmt vet staticcheck boundary \
        constitution fuzz clean \
        android ios coverage

all: lint test build

build:
	$(GO) build $(GOFLAGS) -o $(BIN_DIR)/vayumail ./cmd/vayumail
	$(GO) build $(GOFLAGS) -o $(BIN_DIR)/vayumail-cli ./cmd/vayumail-cli

cli:
	$(GO) build $(GOFLAGS) -o $(BIN_DIR)/vayumail-cli ./cmd/vayumail-cli

test:
	$(GO) test -count=1 -timeout=120s ./...

race:
	$(GO) test -race -count=1 -timeout=120s ./...

coverage:
	$(GO) test -race -count=1 -coverprofile=coverage.txt \
		./internal/mail/... ./internal/store/... ./internal/syncmanager/...
	$(GO) tool cover -func=coverage.txt | tail -1

fmt:
	gofmt -l -w .

vet:
	$(GO) vet ./...

staticcheck:
	$(GO) run honnef.co/go/tools/cmd/staticcheck@latest ./...

# Constitutional Rule 4: engine packages never import Gio.
boundary:
	@result=$$(grep -rl "gioui.org" internal/mail internal/store \
		internal/syncmanager 2>/dev/null || true); \
	if [ -n "$$result" ]; then \
		echo "BOUNDARY VIOLATION: $$result"; exit 1; \
	else \
		echo "package boundary clean"; \
	fi

# All ten constitutional rules, mechanically enforced.
constitution:
	sh scripts/constitution.sh

# Parser hardening: fuzz the attacker-facing MIME surface.
fuzz:
	$(GO) test -run=NONE -fuzz=FuzzMIMEParse -fuzztime=30s ./test/
	$(GO) test -run=NONE -fuzz=FuzzHTMLToText -fuzztime=15s ./test/

lint: vet boundary constitution
	@files=$$(gofmt -l .); \
	if [ -n "$$files" ]; then echo "Unformatted: $$files"; exit 1; fi

# Android APK via gogio. Requires Android SDK/NDK on PATH.
icon:
	go run ./tools/appicon

android:
	$(GO) run gioui.org/cmd/gogio@latest -target android \
		-appid org.vayumail.mobile -o $(BIN_DIR)/vayumail.apk ./cmd/vayumail

# iOS app via gogio. Requires Xcode toolchain on macOS.
ios:
	$(GO) run gioui.org/cmd/gogio@latest -target ios \
		-appid org.vayumail.mobile -o $(BIN_DIR)/vayumail.app ./cmd/vayumail

clean:
	rm -rf $(BIN_DIR) coverage.txt
