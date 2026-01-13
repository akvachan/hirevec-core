.PHONY: run watch test test-watch

run:
	go run ./cmd/server/main.go

watch:
	watchexec -e go -r -c --delay-run=11 --stop-timeout=11 -- make run

test:
	go test -v ./tests

test-watch:
	watchexec -w tests -e go -r -c -- make test
