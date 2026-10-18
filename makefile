# recipes use bash for pipefail support (ubuntu's default sh is dash)
SHELL := /bin/bash

# the short commit, as a release stamps it; whether the tree was changed is in the version
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null)
# falls back to dev when there is no tag yet (the || must wrap git, not sed, or it never fires)
GIT_VERSION=$(shell (git describe --tags --dirty 2>/dev/null || echo dev) | sed 's/-\([0-9]*\)-g/+\1@g/')
TEST_TIMEOUT?=5m

# dev tool binaries are built into .tools/bin (gitignored) from the versions pinned in
# .tools/go.mod - the single source of truth for make and CI; dependabot keeps them updated.
# overridable because binaries cannot execute from a noexec mount (e.g. an smb checkout):
# make TOOLS_BIN=~/.cache/truenas-exporter/bin lint
TOOLS_BIN?=.tools/bin
ACTIONLINT=$(TOOLS_BIN)/actionlint
GOFUMPT=$(TOOLS_BIN)/gofumpt
GOLANGCI_LINT=$(TOOLS_BIN)/golangci-lint

# non-Go tools also live in .tools/bin at pinned versions, but the pins are here (dependabot
# cannot bump them): shellcheck, typos and zizmor are static binaries downloaded from their github
# releases, yamllint is pip installed into a repo-local venv and markdownlint-cli2 is npm installed
# into a repo-local prefix. all rebuild when this makefile changes.
MARKDOWNLINT_CLI2_VERSION=0.23.3
SHELLCHECK_VERSION=v0.11.0
TYPOS_VERSION=v1.51.1
YAMLLINT_VERSION=1.38.0
ZIZMOR_VERSION=v1.30.1
MARKDOWNLINT=$(TOOLS_BIN)/markdownlint-cli2
SHELLCHECK=$(TOOLS_BIN)/shellcheck
TYPOS=$(TOOLS_BIN)/typos
YAMLLINT=$(TOOLS_BIN)/yamllint
ZIZMOR=$(TOOLS_BIN)/zizmor

# golangci-lint with the azproviderlint module plugin compiled in (.tools/.custom-gcl.yml);
# lint runs use this binary, the plain go.mod one exists to bootstrap `golangci-lint custom`
GOLANGCI_LINT_MODULES=$(TOOLS_BIN)/golangci-with-modules

# one rule builds any Go tool: the import path comes from the tool directives in .tools/go.mod
# (via go list tool), so the makefile never repeats it - add a tool there and a variable above
$(TOOLS_BIN)/%: .tools/go.mod .tools/go.sum
	@echo "==> building $* (version pinned in .tools/go.mod)..."
	@mkdir -p $(TOOLS_BIN)
	@cd .tools && go build -o $(abspath $@) $$(go list tool | grep "/$*$$")

# explicit rules take precedence over the pattern rule above for the non-Go tools.
# `golangci-lint custom` always writes to .tools/bin (destination in .custom-gcl.yml), so move
# the result when TOOLS_BIN points elsewhere (e.g. a local dir because the checkout is noexec)
$(GOLANGCI_LINT_MODULES): .tools/.custom-gcl.yml $(GOLANGCI_LINT)
	@echo "==> building golangci-lint with plugins (versions pinned in .tools/.custom-gcl.yml)..."
	@cd .tools && mkdir -p bin && $(abspath $(GOLANGCI_LINT)) custom
	@if [ "$(abspath $(TOOLS_BIN))" != "$(abspath .tools/bin)" ]; then mv .tools/bin/golangci-with-modules $@; fi

$(SHELLCHECK): makefile
	@echo "==> downloading shellcheck $(SHELLCHECK_VERSION)..."
	@mkdir -p $(TOOLS_BIN)
	@os=$$(uname | tr 'A-Z' 'a-z'); arch=$$(uname -m); [ "$$arch" = "arm64" ] && arch=aarch64; \
		curl -sSfL "https://github.com/koalaman/shellcheck/releases/download/$(SHELLCHECK_VERSION)/shellcheck-$(SHELLCHECK_VERSION).$$os.$$arch.tar.xz" \
		| tar -xJ -O shellcheck-$(SHELLCHECK_VERSION)/shellcheck > $@ && chmod +x $@

$(YAMLLINT): makefile
	@command -v python3 >/dev/null || (echo "python3 is required to install yamllint (macOS: xcode CLT; Debian/Ubuntu: apt install python3-venv)" && exit 1)
	@echo "==> installing yamllint $(YAMLLINT_VERSION) into $(TOOLS_BIN)/../venv..."
	@mkdir -p $(TOOLS_BIN)
	@python3 -m venv $(TOOLS_BIN)/../venv && $(TOOLS_BIN)/../venv/bin/pip install -q yamllint==$(YAMLLINT_VERSION) && ln -sf ../venv/bin/yamllint $@

$(ZIZMOR): makefile
	@echo "==> downloading zizmor $(ZIZMOR_VERSION)..."
	@mkdir -p $(TOOLS_BIN)
	@case "$$(uname)" in Darwin) target=apple-darwin;; *) target=unknown-linux-gnu;; esac; \
		arch=$$(uname -m); [ "$$arch" = "arm64" ] && arch=aarch64; \
		curl -sSfL "https://github.com/zizmorcore/zizmor/releases/download/$(ZIZMOR_VERSION)/zizmor-$$arch-$$target.tar.gz" \
		| tar -xz -O zizmor > $@ && chmod +x $@

$(TYPOS): makefile
	@echo "==> downloading typos $(TYPOS_VERSION)..."
	@mkdir -p $(TOOLS_BIN)
	@case "$$(uname)" in Darwin) target=apple-darwin;; *) target=unknown-linux-musl;; esac; \
		arch=$$(uname -m); [ "$$arch" = "arm64" ] && arch=aarch64; \
		curl -sSfL "https://github.com/crate-ci/typos/releases/download/$(TYPOS_VERSION)/typos-$(TYPOS_VERSION)-$$arch-$$target.tar.gz" \
		| tar -xz -O ./typos > $@ && chmod +x $@

$(MARKDOWNLINT): makefile
	@command -v npm >/dev/null || (echo "npm is required to install markdownlint-cli2 (macOS: brew install node; Debian/Ubuntu: apt install npm)" && exit 1)
	@echo "==> installing markdownlint-cli2 $(MARKDOWNLINT_CLI2_VERSION) into $(TOOLS_BIN)/../npm..."
	@mkdir -p $(TOOLS_BIN)
	@npm install --silent --no-audit --no-fund --prefix $(TOOLS_BIN)/../npm markdownlint-cli2@$(MARKDOWNLINT_CLI2_VERSION) && ln -sf ../npm/node_modules/.bin/markdownlint-cli2 $@ && touch $@

default: fmt build

all: fmt build

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make \033[36m<target>\033[0m\n"} /^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-24s\033[0m%s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)

##@ Build
build: ## Compile truenas-exporter with version info from git
	@echo "==> building..."
	go build -ldflags "-X github.com/katbyte/go-kt/version.GitCommit=${GIT_COMMIT} -X github.com/katbyte/go-kt/version.Version=${GIT_VERSION}"

install: ## Install truenas-exporter into GOPATH/bin with version info from git
	@echo "==> installing..."
	go install -ldflags "-X github.com/katbyte/go-kt/version.GitCommit=${GIT_COMMIT} -X github.com/katbyte/go-kt/version.Version=${GIT_VERSION}" .

# the base images come from Google's mirror of Docker Hub, and from Docker Hub itself when the mirror gives nothing, as the build workflow does it
docker: ## Build the truenas-exporter container image with version info from git
	@echo "==> building docker image..."
	docker build --build-arg VERSION=${GIT_VERSION} --build-arg COMMIT=${GIT_COMMIT} -t truenas-exporter . || docker build --build-arg REGISTRY=docker.io/library --build-arg VERSION=${GIT_VERSION} --build-arg COMMIT=${GIT_COMMIT} -t truenas-exporter .

tools: $(ACTIONLINT) $(GOFUMPT) $(GOLANGCI_LINT) $(GOLANGCI_LINT_MODULES) $(MARKDOWNLINT) $(SHELLCHECK) $(TYPOS) $(YAMLLINT) $(ZIZMOR) ## Install all pinned dev tools into .tools/bin

##@ Formatting
fmt: $(GOFUMPT) $(GOLANGCI_LINT) ## Fix Go formatting (gofmt, gofumpt, goimports)
	@echo "==> Fixing source code with gofmt..."
	find . -name '*.go' | grep -v -e vendor -e '\.testnas' | xargs gofmt -s -w
	@echo "==> Fixing source code with gofumpt..."
	find . -name '*.go' | grep -v -e vendor -e '\.testnas' | xargs $(GOFUMPT) -w
	@echo "==> Fixing imports with golangci-lint (goimports)..."
	$(GOLANGCI_LINT) fmt -E goimports ./...

goimports: $(GOLANGCI_LINT) ## Fix imports with golangci-lint (goimports)
	@echo "==> Fixing imports with golangci-lint (goimports)..."
	$(GOLANGCI_LINT) fmt -E goimports ./...

##@ Linting & Dependencies
# markdown checked by markdownlint: every .md the repo owns
MARKDOWN_INPUTS='*.md' 'docs/**/*.md' 'lib/nas/testdata/README.md' '.github/**/*.md'

lint: $(GOLANGCI_LINT_MODULES) ## Check source code with the golangci linters (incl. azproviderlint)
	@echo "==> Checking source code against linters..."
	$(GOLANGCI_LINT_MODULES) run ./...

actionlint: $(ACTIONLINT) $(SHELLCHECK) ## Check GitHub workflows with actionlint (incl. shellcheck on run blocks)
	@echo "==> Checking workflows with actionlint..."
	@$(ACTIONLINT) -shellcheck=$(SHELLCHECK)

lint-fix: $(GOLANGCI_LINT_MODULES) ## Fix source code with all golangci linters
	@echo "==> Checking source code against linters (applying autofixes)..."
	$(GOLANGCI_LINT_MODULES) run --fix ./...

yamllint: $(YAMLLINT) ## Check YAML files with yamllint (config in .yamllint.yml)
	@echo "==> Checking YAML files with yamllint..."
	@$(YAMLLINT) -s .

shellcheck: $(SHELLCHECK) ## Check shell scripts with shellcheck
	@echo "==> Checking shell scripts with shellcheck..."
	@$(SHELLCHECK) .scripts/*.sh

zizmor: $(ZIZMOR) ## Audit GitHub workflows for security issues with zizmor
	@echo "==> Auditing workflows with zizmor..."
	@$(ZIZMOR) .

typos: $(TYPOS) ## Check spelling in code and docs with typos (config in .typos.toml)
	@echo "==> Checking spelling with typos..."
	@$(TYPOS) || \
		(echo; echo "Spelling errors found. Fix them with 'make typos-fix', or add false positives (names, API words) to .typos.toml."; exit 1)

typos-fix: $(TYPOS) ## Fix spelling errors found by typos
	@$(TYPOS) --write-changes

markdownlint: $(MARKDOWNLINT) ## Check the repo's markdown with markdownlint (config in .markdownlint.yml)
	@echo "==> Checking markdown with markdownlint..."
	@$(MARKDOWNLINT) $(MARKDOWN_INPUTS)

depscheck: ## Check that go.mod/go.sum and vendor/ are in sync
	@echo "==> Checking source code with go mod tidy..."
	@go mod tidy
	@git diff --exit-code -- go.mod go.sum || \
		(echo; echo "Unexpected difference in go.mod/go.sum files. Run 'go mod tidy' command or revert any go.mod/go.sum changes and commit."; exit 1)
	@echo "==> Checking source code with go mod vendor..."
	@go mod vendor
	@git diff --compact-summary --exit-code -- vendor || \
		(echo; echo "Unexpected difference in vendor/ directory. Run 'go mod vendor' command or revert any go.mod/go.sum/vendor changes and commit."; exit 1)
	@echo "==> Checking .tools/go.mod with go mod tidy..."
	@cd .tools && go mod tidy
	@git diff --exit-code -- .tools/go.mod .tools/go.sum || \
		(echo; echo "Unexpected difference in .tools/go.mod/go.sum. Run 'cd .tools && go mod tidy' and commit."; exit 1)
	@echo "==> Checking .tools/.custom-gcl.yml golangci-lint version matches .tools/go.mod..."
	@modv=$$(cd .tools && go list -m -f '{{.Version}}' github.com/golangci/golangci-lint/v2); \
		gclv=$$(grep '^version:' .tools/.custom-gcl.yml | awk '{print $$2}'); \
		[ "$$modv" = "$$gclv" ] || \
		(echo; echo "golangci-lint version mismatch: .tools/go.mod has $$modv but .tools/.custom-gcl.yml has $$gclv - update .custom-gcl.yml to match."; exit 1)

##@ Testing
# every test runs against a NAS the test makes up (lib/poll/polltest): none needs a TrueNAS
test: build ## Run the tests under the race detector
	go test -race ./... -timeout ${TEST_TIMEOUT}

# the live suite (acceptance/) runs the program against a real TrueNAS and checks the page against what that NAS says when asked directly. it only
# reads, so it can be pointed at any NAS: a throwaway one in a virtual machine for choice, where a pool can be broken on purpose. the NAS is named
# by four variables, kept out of git in a .testenv.sh to source if that is easier: . ./.testenv.sh && make test-live
test-live: ## Run the live tests against the TrueNAS named by TRUENAS_EXPORTER_TEST_SERVER, _USERNAME, _API_KEY and _CERTIFICATE_FILE
	@[ -n "${TRUENAS_EXPORTER_TEST_SERVER}" ] && [ -n "${TRUENAS_EXPORTER_TEST_USERNAME}" ] && [ -n "${TRUENAS_EXPORTER_TEST_API_KEY}" ] || \
		(echo 'TRUENAS_EXPORTER_TEST_SERVER, TRUENAS_EXPORTER_TEST_USERNAME and TRUENAS_EXPORTER_TEST_API_KEY must be set, and TRUENAS_EXPORTER_TEST_CERTIFICATE_FILE to the certificate of a NAS the system does not already trust'; exit 1)
	go test -tags acceptance -count=1 ./acceptance/ -timeout ${TEST_TIMEOUT} -v

# the test NAS is truenas-mcp's to make, so its test environment is fetched into .testnas (gitignored) at the version of truenas-mcp this program
# is built on: the tag go.mod names, or the commit at the end of a version that has no tag. TRUENAS_MCP names a checkout to use as it is instead
TRUENAS_MCP?=.testnas

testnas: ## Fetch truenas-mcp's test environment into .testnas, at the version of truenas-mcp in go.mod
	@echo "==> fetching the test environment..."
	@set -e; \
		ref=$$(go list -mod=mod -m -f '{{.Version}}' github.com/katbyte/truenas-mcp | sed -E 's/^.*[-.][0-9]{14}-([0-9a-f]{12})$$/\1/'); \
		[ -d .testnas/.git ] || git clone --quiet https://github.com/katbyte/truenas-mcp.git .testnas; \
		git -C .testnas fetch --quiet --tags origin; \
		git -C .testnas checkout --quiet "$$ref"; \
		git -C .testnas log --oneline -1

# testacc runs the live tests against a throwaway TrueNAS of its own: the test environment installs one in a virtual machine and sets it up with a
# pool and a read-only user, the suite is run against it as that user, and the machine is removed. TRUENAS_TEST_FURNISHED=1 asks the test
# environment to also give the NAS a share of each kind, a virtual machine and tasks that ran, never ran and failed, and tells the suite to look
# for them. it needs qemu and an x86-64 Linux machine with /dev/kvm, which a GitHub runner is and a Mac is not: the acceptance workflow runs it.
# A run that names no NAS stops, since the live tests would otherwise only skip and pass.
testacc: $(if $(filter .testnas,$(TRUENAS_MCP)),testnas) ## Run the live tests against a throwaway TrueNAS in a virtual machine (needs qemu and /dev/kvm)
	@[ -x "$(TRUENAS_MCP)/.scripts/testenv.sh" ] || (echo 'no truenas-mcp checkout at $(TRUENAS_MCP): its .scripts/testenv.sh is what makes the test NAS'; exit 1)
	@echo "==> live tests..."
	@set -e; \
		env="$(CURDIR)/.testenv.sh"; \
		export TRUENAS_TEST_FURNISHED=1; \
		trap 'st=$$?; [ $$st -eq 0 ] || $(TRUENAS_MCP)/.scripts/testenv.sh logs || true; $(TRUENAS_MCP)/.scripts/testenv.sh down; rm -f '"$$env"'; exit $$st' EXIT; \
		$(TRUENAS_MCP)/.scripts/testenv.sh up > "$$env"; \
		. "$$env"; \
		[ -n "$$TRUENAS_EXPORTER_TEST_SERVER" ] && [ -n "$$TRUENAS_EXPORTER_TEST_API_KEY" ] || { echo 'testenv.sh up named no NAS to test against, so the live tests would only have skipped'; exit 1; }; \
		go test -tags acceptance -count=1 ./acceptance/ -timeout ${TEST_TIMEOUT} -v

# coverage is written under .coverage (gitignored); the coverage workflow reads the total
# from coverage.out and publishes it as the README badge. it is counted across packages, since
# the commands' tests are what run the poller and the page end to end
COVERDIR?=.coverage
COVERPKG=./...

cover: build ## Run the tests with coverage and report the total
	@rm -rf $(COVERDIR) && mkdir -p $(COVERDIR)
	go test -race -count=1 -coverpkg=$(COVERPKG) -coverprofile=$(COVERDIR)/coverage.out ./... -timeout ${TEST_TIMEOUT}
	@go tool cover -func=$(COVERDIR)/coverage.out | tail -1

cover-html: cover ## Run the tests with coverage and open the HTML report
	@go tool cover -html=$(COVERDIR)/coverage.out

check-all: build test lint actionlint zizmor yamllint shellcheck typos markdownlint depscheck ## Run build + tests + all linters + depscheck

.PHONY: default all help fmt goimports build install docker lint lint-fix actionlint yamllint shellcheck zizmor typos typos-fix markdownlint depscheck check-all tools test test-live testnas testacc cover cover-html
