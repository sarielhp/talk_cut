# talk_cut - Talk video cutting & YouTube publishing pipeline
# Delegates to the standard repository tools (see tools/settings.rb).
# Use tools/commit directly for messages with shell metacharacters.

.PHONY: all build gate check check-full ci lint snapshot wip commit bump install release test test-tui test-scroll test-vhs audit review static-analysis clean

BIN_NAME := talk_cut

all: build
build:
	tools/build
gate:
	tools/gate
check:
	tools/check
check-full:
	tools/gate --full
ci:
	tools/gate --full
lint:
	tools/lint
snapshot:
	tools/snapshot $(ARGS)
wip:
	tools/snapshot $(if $(msg),"wip: $(msg)",)
commit:
	tools/commit "$(msg)"
bump:
	tools/bump
install:
	tools/install
release:
	tools/release $(ARGS)
test:
	go test -timeout 180s ./...

# Live interactive TUI snapshot tests (headless tmux)
test-tui: build
	ruby tools/live_tui_test.rb

# Live full transcript scroll test (top & bottom invariants across 540 cues)
test-scroll: build
	ruby tools/live_tui_test.rb --scroll-only

# Full live test, read-only (hard lock): CLI, TUI tasks, playlist editor, random stress.
# Slow and talks to YouTube; not part of the gate. ARGS, e.g. ARGS="--steps 1000 --seed 7".
live-test:
	tools/live_test $(ARGS)

# Docker-based Charm VHS snapshot testing
test-vhs: build
	docker run --rm -v "$$(pwd):/vhs" ghcr.io/charmbracelet/vhs tools/test_tui.tape

audit:
	go-audit --strict --quiet .
review: static-analysis
static-analysis:
	go-static-analysis
clean:
	go clean
	rm -f $(BIN_NAME)
