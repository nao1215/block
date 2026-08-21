.PHONY: build test e2e doc doc-check demo favicon registry-verify registry-sync registry-live examples-live coverage clean vet fmt lint website website-serve changelog help

APP         = block
VERSION     = $(shell git describe --tags --abbrev=0 2>/dev/null || echo dev)
GO          = go
GO_BUILD    = $(GO) build
GO_FORMAT   = $(GO) fmt
GOFMT       = gofmt
GO_LIST     = $(GO) list
GO_TEST     = $(GO) test -v
GO_TOOL     = $(GO) tool
GO_VET      = $(GO) vet
GOOS        = ""
GOARCH      = ""
GO_PKGROOT  = ./...
GO_PACKAGES = $(shell $(GO_LIST) $(GO_PKGROOT))
GO_LDFLAGS  = -ldflags '-X github.com/nao1215/block/internal/cmdinfo.Version=${VERSION}'

build:  ## Build binary
	env GO111MODULE=on GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO_BUILD) $(GO_LDFLAGS) -o $(APP) main.go

clean: ## Clean project
	-rm -rf $(APP) cover.out cover.html .coverage dist

test: ## Start test
	env GOOS=$(GOOS) $(GO_TEST) -race -cover $(GO_PKGROOT) -coverprofile=cover.out
	$(GO_TOOL) cover -html=cover.out -o cover.html

e2e: ## Run offline end-to-end tests against the real CLI (requires atago)
	./e2e/run.sh

doc: ## Regenerate the derived docs (doc/tools.md, doc/errors.md)
	$(GO) run ./scripts/gen-docs

doc-check: ## Fail if a generated doc is stale (offline; run by CI)
	$(GO) run ./scripts/gen-docs -check

demo: build ## Re-record the README GIFs (requires vhs and ffmpeg; needs network and GITHUB_TOKEN)
	mkdir -p dist && cp $(APP) dist/$(APP)
	env BLOCK_HOME=/tmp/block-demo sh -c 'for d in doc/demo/project doc/demo/defi doc/demo/bridge; do \
	  (cd "$$d" && rm -f block.lock && ../../../dist/$(APP) lock >/dev/null && ../../../dist/$(APP) sync >/dev/null); \
	done'
	vhs doc/vhs/demo.tape
	vhs doc/vhs/shims.tape
	vhs doc/vhs/list.tape
	# The social card is the last frame of the hero GIF: -update rewrites the
	# same file for every frame, so the one left behind is the final screen.
	ffmpeg -v error -y -i doc/img/demo.gif -update 1 doc/img/social.png

favicon: ## Redraw doc/img/favicon.png, the one image a recording cannot be
	python3 ./scripts/gen-favicon.py

registry-verify: ## Check that registry/ is still the block-registry snapshot it records (offline)
	$(GO) run ./scripts/registry-snapshot -verify

registry-sync: ## Vendor block-registry's recipes into registry/ (network; REVISION=<sha> pins one)
	./scripts/registry-sync.sh

registry-live: ## Check every registry recipe against the real upstreams (downloads artifacts; RECIPE=foundry limits it)
	$(GO) test -tags=live -v -timeout 50m ./registry/ -run 'TestLiveRegistry/($(RECIPE))'

examples-live: ## Check that every examples/*.toml still resolves upstream (network; EXAMPLE=evm-contracts limits it)
	$(GO) test -tags=live -v -timeout 20m ./examples/ -run 'TestLiveExamples/($(EXAMPLE))'

coverage: ## Combine unit + E2E coverage into cover.out / cover.html (uses a `go build -cover` block; scratch under .coverage/)
	bash ./scripts/coverage.sh

website: ## Build the documentation website into website/public (requires hugo)
	cd website && hugo --gc --minify --cleanDestinationDir

website-serve: ## Serve the documentation website locally with live reload
	cd website && hugo server

vet: ## Start go vet
	$(GO_VET) $(GO_PACKAGES)

fmt: ## Format go source code
	$(GO_FORMAT) $(GO_PKGROOT)

lint: ## Run golangci-lint (requires golangci-lint v2)
	golangci-lint run ./...

.DEFAULT_GOAL := help
help:
	@grep -E '^[0-9a-zA-Z_-]+[[:blank:]]*:.*?## .*$$' $(MAKEFILE_LIST) | sort \
	| awk 'BEGIN {FS = ":.*?## "}; {printf "\033[1;32m%-15s\033[0m %s\n", $$1, $$2}'
