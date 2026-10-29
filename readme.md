# Hirevec

## About Hirevec

[Hirevec](https://hirevec.com) is a modern job recommendation engine that finds jobs and suitable candidates.

## Quick start

Start server with:

```sh
go run cmd/hvserver/main.go
```

## Recommendation Engine

The recommendation engine currently only employs a content-based approach. There are currently two stages:

1. Filter

   Either via database-powered text search or vector embeddings, compare position text to a candidate profile.

2. Rerank (Optional)

   One can enable a reranker to reduce the pool of candidates further and establish a better ranking, this can be done regradless of which database you use.

## Server Features

All currently available server settings are demonstrated in [./.example.env](./.example.env).
You can configure them in a `.env` file of a current working directory or via environment variables.

## Misc

### Hot-reload server

```sh
air \
  --build.cmd "go build -o bin/app cmd/hvserver/main.go" \
  --build.entrypoint "./bin/app" \
  --build.include_ext "go,sql,env,json" \
  --build.kill_delay "50000000000" \
  --build.log "server.log" \
  --build.send_interrupt true \
  --color never \
  --log.main_only true \
  --log.silent true \
  --misc.clean_on_exit true \
  --screen.clear_on_rebuild true \
  --screen.keep_scroll false \
  --tmp_dir "bin"
```
