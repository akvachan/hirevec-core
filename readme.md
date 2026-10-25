# Hirevec

## About Hirevec

[Hirevec](https://hirevec.com) is a modern job recommendation engine that finds jobs and suitable candidates.

## Quick start

Start server with:

```sh
go run cmd/hvserver/main.go
```

If you want to have some development data:

## Recommendation Engine

The recommendation engine currently only employs a content-based approach. There are currently two stages:

1. Filter

   Either via database-powered text search or vector embeddings, compare position text to a candidate profile.

2. Rerank (Optional)

   One can enable a reranker to reduce the pool of candidates further and establish a better ranking, this can be done regradless of which database you use.

## Server Features

All currently available server settings are demonstrated in [./.example.env](./.example.env).
You can configure them in a `.env` file of a current working directory or via environment variables.

## TODOs

- [ ] Integrate `candidate_experiences` and `positions.skill*` into FTS.
- [ ] Read locations from CSV (country, city, state).
- [ ] Read positions and candidates from CSV (introduce 10 different CSV-scenarios).
- [ ] Create golden dataset of recommendations per each scenario.
- [ ] Evaluate recommendation engine based on Recall, Precision and F1 in all 4 mode mixtures.
- [ ] Implement continuous monitoring and evaluation.
- [ ] Enable and test SMTP relay.
- [ ] Implement password reset.
- [ ] Implement passwordless login via email.
- [ ] Implement 2FA via email.
- [ ] Enable and test Google and Apple SSO.
- [ ] Implement basic chatting via WebSocket: `golang.org/x/net/websocket`.
- [ ] Implement group 1-to-many group chats where one candidate may talk to multiple recruiters.
- [ ] Enable hyperlink or PDF-based candidate profile creation.
- [ ] Enable hyperlink-based position creation.
- [ ] Implement content moderation with decision models.
- [ ] Implement separate `position_benefits` table for positions.
- [ ] Add mandatory `salary_min`, `salary_max`, `salary_currency`, `salary_frequency` fields to the `positions`.
- [ ] Add mandatory `pref_salary_min`, `pref_salary_max`, `pref_salary_currency`, `pref_salary_frequency` fields to `candidates`.
- [ ] Add optional `language_1`, `language_2`, `language_3` fields to `positions`.
- [ ] Add optional `pref_language_1`, `pref_language_2`, `pref_language_3` fields to `candidates`.
- [ ] Add optional `employees_count` to `positions` as an enum.
- [ ] Add optional `company_type` to `positions` as an enum.
- [ ] Add optional `business_model` to `positions` as an enum.
- [ ] Add optional `interview_type` to `positions` as an enum.
- [ ] Create table `skills` and read skills CSV.
- [ ] Create table `position_titles` and read job titles CSV.

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
