.PHONY: build run test-dual test-batch-only test-step-only test-code-batch test-code-only test-l0 test-l1 clean

BIN = bin/server
PORT = 5400
API_KEY ?=
MODEL ?= glm-5.1
BASE_URL ?= https://open.bigmodel.cn/api/paas/v4
PROJECT_DIR ?= .

define require_api_key
	@if [ -z "$(API_KEY)" ]; then \
		echo "ERROR: API_KEY is required for this target."; \
		exit 2; \
	fi
endef

build:
	go build -o $(BIN) ./cmd/server

run: build
	$(BIN) -port $(PORT)

test-dual: build
	$(require_api_key)
	@API_KEY='$(API_KEY)' $(BIN) -mode test -scenario dual -port $(PORT) \
		-model $(MODEL) -base-url $(BASE_URL) -project-dir $(PROJECT_DIR)

test-batch-only: build
	$(require_api_key)
	@API_KEY='$(API_KEY)' $(BIN) -mode test -scenario batch-only -port $(PORT) \
		-model $(MODEL) -base-url $(BASE_URL) -project-dir $(PROJECT_DIR)

test-step-only: build
	$(require_api_key)
	@API_KEY='$(API_KEY)' $(BIN) -mode test -scenario step-only -port $(PORT) \
		-model $(MODEL) -base-url $(BASE_URL) -project-dir $(PROJECT_DIR)

test-code-batch: build
	$(require_api_key)
	@API_KEY='$(API_KEY)' $(BIN) -mode test -scenario code-batch -port $(PORT) \
		-model $(MODEL) -base-url $(BASE_URL) -project-dir $(PROJECT_DIR)

test-code-only: build
	$(require_api_key)
	@API_KEY='$(API_KEY)' $(BIN) -mode test -scenario code-only -port $(PORT) \
		-model $(MODEL) -base-url $(BASE_URL) -project-dir $(PROJECT_DIR)

test-l0: build
	$(require_api_key)
	@API_KEY='$(API_KEY)' $(BIN) -mode test -scenario l0 -port $(PORT) \
		-model $(MODEL) -base-url $(BASE_URL) -project-dir $(PROJECT_DIR)

test-l1: build
	$(require_api_key)
	@API_KEY='$(API_KEY)' $(BIN) -mode test -scenario l1 -port $(PORT) \
		-model $(MODEL) -base-url $(BASE_URL) -project-dir $(PROJECT_DIR)

test-discovery: test-dual

# ── Codex CLI provider targets (no API_KEY needed) ───────────
# Usage: make codex-dual  (uses codex exec with ChatGPT subscription)

codex-%: build
	@API_KEY='codex' $(BIN) -mode test -scenario $* -port $(PORT) \
		-api-key codex -model gpt-5.4 -project-dir $(PROJECT_DIR)

# ── Zhipu Anthropic provider targets ─────────────────────────
# Usage: make glm-dual API_KEY=xxx  (uses Zhipu Coding Plan via Anthropic API)

GLM_BASE_URL ?= https://open.bigmodel.cn/api/anthropic
GLM_MODEL ?= glm-5.1

glm-%: build
	$(require_api_key)
	@API_KEY='$(API_KEY)' $(BIN) -mode test -scenario $* -port $(PORT) \
		-model $(GLM_MODEL) -base-url $(GLM_BASE_URL) -project-dir $(PROJECT_DIR)

clean:
	rm -rf bin/
