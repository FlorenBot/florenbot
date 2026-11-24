
DOCKER_COMPOSE = docker compose

BIN_DIR := bin

.PHONY: up, down, build, run

up:
	$(DOCKER_COMPOSE) --env-file .env.docker  up -d

down:
	$(DOCKER_COMPOSE) --env-file .env.docker down

build:
	./build.sh

build: build-backend build-bot



run:
	go run ./bot

format:
	gofmt -w -s .

run-skip:
	go run ./bot -skip
	
vet:
	go vet ./...

lint:
	golangci-lint run ./...

.PHONY: build build-backend build-bot clean

build-backend:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/backend ./backend/main.go

build-bot:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/bot ./bot

clean:
	rm -rf $(BIN_DIR)