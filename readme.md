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

1. [x] Semantic/lexical similarity between candidate's CV and job posting (high weight, $`S_{cv}`$)
2. [ ] Candidate is based in the same location (high weight, $`H_{loc}`$)
3. [ ] Candidate searches the same working mode (Remote / Hybrid / Office) (high weight, $`H_{mode}`$)
4. [ ] Candidate speaks required working language (high weight, $`H_{lang}`$)
5. [ ] Skills overlap (medium weight, $`S_{skills}`$)
6. [ ] Semantic/lexical similarity between previous candidate's titles and title in the job posting (medium weight, $`S_{title}`$)
7. [ ] Candidate has required years of experience (low weight, $`S_{yoe}`$)
8. [ ] Candidate already worked for the company before (low weight, $`S_{company}`$)

> [!NOTE]
> All implemented features are marked as [x].

$`\text{Score} = \left(0.60 \cdot S_{cv} + 0.15 \cdot S_{skills} + 0.10 \cdot S_{title} + 0.05 \cdot S_{yoe} + 0.02 \cdot S_{company} \right) \left(0.20 + 0.80 \cdot (0.40 \cdot H_{loc} + 0.30 \cdot H_{mode} + 0.30 \cdot H_{lang}) \right)`$

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
