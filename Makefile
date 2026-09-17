# talk_cut - Talk video cutting & YouTube publishing pipeline

.PHONY: all build check ci lint audit review static-analysis test commit bump clean install

BIN_NAME := talk_cut
GO_FILES := $(shell find . -name "*.go" -not -path "./vendor/*")

all: build

build: $(BIN_NAME)

$(BIN_NAME): $(GO_FILES) go.mod
	go build -o $(BIN_NAME) .

install: $(BIN_NAME)
	mkdir -p ~/bin
	cp $(BIN_NAME) ~/bin/

# Fast quality gate: formatting, vet, staticcheck, cognitive audit, unit tests, build
check:
	ruby tools/check.rb

ci: check

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

# Gated commit (runs quality gate first, stages, commits, records .verified_head)
commit:
	ruby tools/commit.rb "$(msg)"

# Increment patch version, verify, commit, and install
bump:
	ruby tools/bump.rb

clean:
	go clean
	rm -f $(BIN_NAME)
