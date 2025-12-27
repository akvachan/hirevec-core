.PHONY: run watch test test-watch

run:
	go run .

watch:
	watchexec -e go -r -c -- make run

test:
	go test -v ./tests

test-watch:
	watchexec -w tests -e go -r -c -- make test
