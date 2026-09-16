.PHONY: test build check

test:
	go test ./...
	cd web && npm test

build:
	go build ./cmd/lrp ./cmd/lrp-server
	cd web && npm run build

check: test build
	docker compose config --quiet
