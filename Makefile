.PHONY: test check lint lint-go lint-frontend fmt fmt-go fmt-frontend fmt-check fmt-check-go fmt-check-frontend integration run migrate worker
GO_PACKAGES := ./cmd/... ./internal/... ./migrations/...

test:
	go test $(GO_PACKAGES)
check: lint fmt-check
	go test -race $(GO_PACKAGES)
lint: lint-go lint-frontend
lint-go:
	go vet $(GO_PACKAGES)
	go tool staticcheck $(GO_PACKAGES)
lint-frontend:
	npm --prefix frontend run lint
fmt: fmt-go fmt-frontend
fmt-go:
	go tool goimports -local minecraft-hosting -w cmd internal migrations
fmt-frontend:
	npm --prefix frontend run format
fmt-check: fmt-check-go fmt-check-frontend
fmt-check-go:
	./scripts/check-go-format.sh
fmt-check-frontend:
	npm --prefix frontend run format:check
integration:
	@test -n "$(TEST_DATABASE_URL)" || (echo 'Set TEST_DATABASE_URL to a dedicated PostgreSQL test database'; exit 1)
	go test -race -count=1 $(GO_PACKAGES)
run:
	go run ./cmd/api
worker:
	go run ./cmd/worker
migrate:
	go run ./cmd/migrate

.PHONY: panel-build panel-init panel-admin panel-up hosting-up wings-up pterodactyl-config pterodactyl-down test-compose
panel-build:
	./scripts/pterodactyl.sh build
panel-init:
	./scripts/pterodactyl.sh init
panel-admin:
	./scripts/pterodactyl.sh admin
panel-up:
	./scripts/pterodactyl.sh panel
hosting-up:
	./scripts/pterodactyl.sh app
wings-up:
	./scripts/pterodactyl.sh wings
pterodactyl-config:
	./scripts/pterodactyl.sh config
pterodactyl-down:
	./scripts/pterodactyl.sh down
test-compose:
	python3 scripts/test-compose.py

.PHONY: minecraft-installer
minecraft-installer:
	docker compose -f docker-compose.yml -f docker-compose.pterodactyl.yml --profile installers build minecraft-installer
