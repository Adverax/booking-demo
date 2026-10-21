# generated_by: forge:harness — do not edit inside the managed block; edit meta/.skills.yml or run /forge:harness
# >>> forge:harness (managed) >>>
# Stack: Go 1.26, pure Go (modernc.org/sqlite, no cgo), single binary.
# No local infrastructure: SQLite is a file on local disk, so there is no docker-compose.yml.
.DEFAULT_GOAL := help

GOVULNCHECK_VERSION ?= v1.9.0
GO_LICENSES_VERSION ?= v2.0.1
TEST_WORKERS ?= $(shell n=$$( (nproc || sysctl -n hw.ncpu) 2>/dev/null ); echo $$(( n > 1 ? n / 2 : 1 )) )
AUDIT_DIR ?= $(CURDIR)/.audit

.PHONY: help build test vet tools audit

help: ## list targets
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-8s %s\n", $$1, $$2}'

build: ## compile all packages
	go build ./...

test: ## unit test suite (worker cap: TEST_WORKERS)
	go test -p $(TEST_WORKERS) ./...

vet: ## go vet static checks
	go vet ./...

tools: ## install the pinned audit tools (govulncheck, go-licenses)
	go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	go install github.com/google/go-licenses/v2@$(GO_LICENSES_VERSION)

audit: ## dependency scan + licence report into AUDIT_DIR (kanban done step 2c)
	@mkdir -p "$(AUDIT_DIR)"; d="$(AUDIT_DIR)"; \
	if ! command -v govulncheck >/dev/null 2>&1; then echo not-installed > "$$d/govulncheck.status"; \
	elif govulncheck -json ./... > "$$d/govulncheck.json" 2> "$$d/govulncheck.err"; then echo ok > "$$d/govulncheck.status"; \
	elif grep -qiE 'dial tcp|no such host|connection refused|i/o timeout|network is unreachable|TLS handshake' "$$d/govulncheck.err"; \
	then echo scanner-unavailable > "$$d/govulncheck.status"; \
	else echo failed > "$$d/govulncheck.status"; fi; \
	if ! command -v go-licenses >/dev/null 2>&1; then echo not-installed > "$$d/licenses.status"; \
	elif go-licenses report ./... > "$$d/licenses.raw" 2> "$$d/licenses.err"; then \
	  awk -F, 'NF>=3 {print $$1","$$NF}' "$$d/licenses.raw" > "$$d/licenses.csv" && echo ok > "$$d/licenses.status"; \
	else echo failed > "$$d/licenses.status"; fi
# <<< forge:harness (managed) <<<
