# Hirevec Core

## About Hirevec
Hirevec is a job recommendation engine.
It helps candidates find suitable positions based on their profile and helps recruiters find suitable candidates.

## Quick Start

```sh
go run cmd/core/main.go
```

## Features

- SQLite with a custom vector search extension under the hood.
- Utilizes `gemini-embedding-001` embeddings for semantic encodig. 
- If offline or no `GOOGLE_API_KEY` provided as environment variable, then uses [Okapi BM25](https://en.wikipedia.org/wiki/Okapi_BM25).

## Notes

Embeddings Job (once every 30 seconds):
1. Select up to N pending or failed embedding jobs with attempts < max_attempts.
2. For each job, load the source data.
3. Batch data and send batches to the embedding serevice.
4. For each embedding, If the source changed during processing, leave status as pending; otherwise upsert and mark completed.
5. On transient failure, leave status as pending.
6. On permanent failure, increment attempts and mark failed if max reached.

Recommendation Job (once every 24 hours):
1. Select candidates with completed embeddings who haven’t been given recommendations today, limited by batch size.
2. Load the candidate’s embedding vector.
3. Perform vector search to find top positions.
4. Filter out inactive or already recommended positions.
5. Fetch position details from the database.
6. Call rerank service with retries on transient failures.
7. Take the top daily_limit positions from reranked results.
8. In a transaction, insert recommendations and update the candidate’s last_recommended_at.
9. On transaction failure, rollback, log, and continue to next candidate.
