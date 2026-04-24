.PHONY: cycle run

cycle:
	docker compose down && docker volume prune --force && docker compose up

run:
	go run ./cmd/demo

