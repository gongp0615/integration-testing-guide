.PHONY: build run test-dual test-batch-only test-step-only test-code-batch test-code-only clean

BIN = bin/server
PORT = 5400
API_KEY ?= $(or $(API_KEY),)
MODEL ?= glm-5.1
BASE_URL ?= https://open.bigmodel.cn/api/paas/v4
PROJECT_DIR ?= .

build:
	go build -o $(BIN) ./cmd/server

run: build
	$(BIN) -port $(PORT)

test-dual: build
	$(BIN) -mode test -scenario dual -port $(PORT) \
		-api-key $(API_KEY) -model $(MODEL) -base-url $(BASE_URL) -project-dir $(PROJECT_DIR)

test-batch-only: build
	$(BIN) -mode test -scenario batch-only -port $(PORT) \
		-api-key $(API_KEY) -model $(MODEL) -base-url $(BASE_URL) -project-dir $(PROJECT_DIR)

test-step-only: build
	$(BIN) -mode test -scenario step-only -port $(PORT) \
		-api-key $(API_KEY) -model $(MODEL) -base-url $(BASE_URL) -project-dir $(PROJECT_DIR)

test-code-batch: build
	$(BIN) -mode test -scenario code-batch -port $(PORT) \
		-api-key $(API_KEY) -model $(MODEL) -base-url $(BASE_URL) -project-dir $(PROJECT_DIR)

test-code-only: build
	$(BIN) -mode test -scenario code-only -port $(PORT) \
		-api-key $(API_KEY) -model $(MODEL) -base-url $(BASE_URL) -project-dir $(PROJECT_DIR)

test-discovery: test-dual

clean:
	rm -rf bin/
