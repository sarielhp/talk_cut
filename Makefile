# talk_cut - Talk video cutting & YouTube publishing pipeline

.PHONY: all build check check-full full-gate gate-full ci lint audit review static-analysis test test-tui test-scroll commit wip bump bump-minor bump-major clean install

BIN_NAME := talk_cut
GO_FILES := $(shell find . -name "*.go" -not -path "./vendor/*")

all: build

build: $(BIN_NAME)

$(BIN_NAME): $(GO_FILES) go.mod
	go build -o $(BIN_NAME) .
	ln -sf $(BIN_NAME) talk_cal

install: $(BIN_NAME)
	mkdir -p ~/bin
	cp $(BIN_NAME) ~/bin/
	ln -sf ~/bin/$(BIN_NAME) ~/bin/talk_cal

# Fast quality gate: formatting, vet, staticcheck, cognitive audit, unit tests, build
check:
	ruby tools/check.rb

ci: check

# Full quality gate: fast gate + deep static-analysis + live TUI tests + full 540-cue scroll test
check-full: build
	ruby tools/check.rb --full

full-gate: check-full
gate-full: check-full

# Sizing & cognitive complexity audit
audit:
	go-audit

# Fast static linting
lint:
	go vet ./...
	staticcheck ./...
	go-audit --quiet

# Deep multi-tool static analysis (gocritic, shadow, revive, govulncheck, dupl)
review: static-analysis
static-analysis:
	go-static-analysis

# Unit tests
test:
	go test -v ./...

# Live interactive TUI snapshot tests (headless tmux)
test-tui: build
	ruby tools/live_tui_test.rb

# Live full transcript scroll test (verifies top & bottom invariants across 540 cues)
test-scroll: build
	ruby tools/live_tui_test.rb --scroll-only

# Docker-based Charm VHS snapshot testing
test-vhs: build
	docker run --rm -v "$$(pwd):/vhs" ghcr.io/charmbracelet/vhs tools/test_tui.tape

# Gated commit (runs quality gate first, stages, commits, records .verified_head)
commit:
	ruby tools/commit.rb "$(msg)"

# Quick WIP commit (auto-generates summary if no msg provided)
wip:
	ruby tools/commit.rb $(if $(msg),"wip: $(msg)","")

# Increment patch version (0.0.1 -> 0.0.2), verify, commit, and install
bump:
	ruby tools/bump.rb patch "$(msg)"

# Increment minor version (0.0.X -> 0.1.0), verify, commit, and install
bump-minor:
	ruby tools/bump.rb minor "$(msg)"

# Increment major version (0.X.Y -> 1.0.0), verify, commit, and install
bump-major:
	ruby tools/bump.rb major "$(msg)"

clean:
	go clean
	rm -f $(BIN_NAME)
