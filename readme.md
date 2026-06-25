# Hirevec

## About Hirevec

Hirevec is a modern job recommendation engine that finds jobs and suitable candidates.

- Homepage: hirevec.com
- API: api.hirevec.com

## Quick Start

1. Start server:

```sh
go run cmd/hv-server/main.go
```

2. Ingest some test data and login as a test user:

```sh
go run cmd/hv-cli/main.go quick-start
```

3. List recommendations for the logged in user:

```sh
go run cmd/hv-cli/main.go recommendations
```

4. React to the first recommendation:

- Positively:

```sh
go run cmd/hv-cli/main.go positive <recommendation_id>
```

- Negatively:

```
go run cmd/hv-cli/main.go negative <recommendation_id>
```

5. See all your matches:

```sh
go run cmd/hv-cli/main.go matches
```

A match occurs when a candidate applies for a position
and a recruiter that created this position, shortlists the candidate.

### Recommendation Engine

The recommendation engine currently only employs a content-based approach.
There are plans to extend it to utilize collaborative filtering with matrix factorization.

#### Model Features

| Variable        | Meaning                       |
| --------------- | ----------------------------- |
| $`S_{bm25}`$    | Normalized BM25/FTS score     |
| $`S_{embed}`$   | Embedding cosine similarity   |
| $`S_{rerank}`$  | Reranker score                |
| $`S_{skills}`$  | Skills overlap                |
| $`S_{title}`$   | Previous-title similarity     |
| $`S_{yoe}`$     | Years-of-experience match     |
| $`S_{company}`$ | Worked for company before     |
| $`H_{loc}`$     | Location match (0/1)          |
| $`H_{mode}`$    | Work mode match (0/1)         |
| $`H_{lang}`$    | Language match (0/1)          |
| $`\alpha`$      | 1 if reranker enabled, else 0 |
| $`\beta`$       | BM25/embedding mixture weight |

$`Score=\left(0.8\left(\alpha S_{rerank}+(1-\alpha)\left(\beta S_{embed}+(1-\beta)S_{bm25}\right)\right)+0.2\left(0.6S_{skills}+0.25S_{title}+0.1S_{yoe}+0.05S_{company}\right)\right)\times\left(0.2+0.8\left(0.4H_{loc}+0.3H_{mode}+0.3H_{lang}\right)\right)`$

## Server Features

All currently available server settings are demonstrated in [./.example.env](./.example.env).
You can define them in a `.env` file or via environment variables.

Following paths will be searched for a `.env`:

- Current working directory
- `~/.config/hv-server`

### Checklist

- [ ] Users can enable PostgreSQL
- [ ] Users can enable TEI (embeddings and reranker worker)
- [ ] Users can enable SMTP relay for security-critical operations
- [ ] Users can enable SSO with Google and Apple

## Misc

### Hot-reload server

```sh
air \
  --build.cmd "go build -o bin/app cmd/hv-server/main.go" \
  --build.entrypoint "./bin/app" \
  --build.include_ext "go,sql,env" \
  --misc.clean_on_exit true \
  --log.main_only true \
  --tmp_dir "bin" \
  --misc.clean_on_exit true \
  --log.silent true \
  --screen.clear_on_rebuild true \
  --color never \
  --screen.keep_scroll false
```
