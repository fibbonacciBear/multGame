CLIENT_DIR := client
SERVER_DIR := server
API_DIR := api

.PHONY: build test run lint compose-down

build:
	npm --prefix $(CLIENT_DIR) run build
	cd $(SERVER_DIR) && go build ./...
	cd $(API_DIR) && go build ./...

test:
	cd $(SERVER_DIR) && go test ./...
	cd $(API_DIR) && go test ./...
	npm --prefix $(CLIENT_DIR) run lint

run:
	docker compose up --build

lint:
	cd $(SERVER_DIR) && go vet ./...
	cd $(API_DIR) && go vet ./...
	npm --prefix $(CLIENT_DIR) run lint

compose-down:
	docker compose down --remove-orphans
