CLIENT_DIR := client
SERVER_DIR := server
API_DIR := api
WS_ROUTER_DIR := ws-router

.PHONY: build test run lint compose-down k8s-dev k8s-prod k8s-dev-monitoring k8s-prod-monitoring k8s-argocd k3d-up k3d-down k8s-apply-dev k8s-apply-dev-monitoring helm-metrics-dev helm-observability load-images-dev typecheck

build:
	npm --prefix $(CLIENT_DIR) run build
	cd $(SERVER_DIR) && go build ./...
	cd $(API_DIR) && go build ./...
	cd $(WS_ROUTER_DIR) && go build ./...

typecheck:
	npm --prefix $(CLIENT_DIR) run typecheck

test:
	cd $(SERVER_DIR) && go test ./...
	cd $(API_DIR) && go test ./...
	cd $(WS_ROUTER_DIR) && go test ./...
	npm --prefix $(CLIENT_DIR) run test

run:
	docker compose up --build

lint:
	cd $(SERVER_DIR) && golangci-lint run ./...
	cd $(API_DIR) && golangci-lint run ./...
	cd $(WS_ROUTER_DIR) && golangci-lint run ./...
	npm --prefix $(CLIENT_DIR) run lint

compose-down:
	docker compose down --remove-orphans

k8s-dev:
	kubectl kustomize k8s/overlays/dev

k8s-prod:
	kubectl kustomize k8s/overlays/prod

k8s-dev-monitoring:
	kubectl kustomize k8s/overlays/dev-monitoring

k8s-prod-monitoring:
	kubectl kustomize k8s/overlays/prod-monitoring

k8s-argocd:
	kubectl kustomize k8s/argocd

k3d-up:
	k3d cluster create multgame --agents 1 --port "8080:80@loadbalancer"

k3d-down:
	k3d cluster delete multgame

load-images-dev:
	docker build -t multgame/game-server:latest ./server
	docker build -t multgame/api-server:latest ./api
	docker build -t multgame/ws-router:latest ./ws-router
	docker build --build-arg VITE_SPECTATOR_MODE_ENABLED=true -t multgame/web-client:latest ./client
	k3d image import multgame/game-server:latest -c multgame
	k3d image import multgame/api-server:latest -c multgame
	k3d image import multgame/ws-router:latest -c multgame
	k3d image import multgame/web-client:latest -c multgame

helm-metrics-dev:
	helm upgrade --install kube-prometheus-stack prometheus-community/kube-prometheus-stack -n monitoring --create-namespace -f k8s/helm/kube-prometheus-stack-values.yaml
	helm upgrade --install prometheus-adapter prometheus-community/prometheus-adapter -n monitoring -f k8s/helm/prometheus-adapter-values.yaml

helm-observability: helm-metrics-dev
	helm upgrade --install loki grafana/loki -n monitoring -f k8s/helm/loki-values.yaml
	helm upgrade --install promtail grafana/promtail -n monitoring -f k8s/helm/promtail-values.yaml

k8s-apply-dev:
	kubectl apply -k k8s/overlays/dev

k8s-apply-dev-monitoring:
	kubectl apply -k k8s/overlays/dev-monitoring
