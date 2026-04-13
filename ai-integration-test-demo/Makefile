.PHONY: build run test-basic test-cross test-edge clean

BIN = bin/server
PORT = 5400

build:
	go build -o $(BIN) ./cmd/server

run: build
	$(BIN) -port $(PORT)

test-basic: build
	$(BIN) -mode test -scenario basic -port $(PORT)

test-cross: build
	$(BIN) -mode test -scenario cross-module -port $(PORT)

test-edge: build
	$(BIN) -mode test -scenario edge-case -port $(PORT)

clean:
	rm -rf bin/
