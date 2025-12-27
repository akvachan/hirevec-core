## DB tables

### Positions
Primary key: `position_id`
Fields: `title`, `company`, `description`

### Candidates
Primary key: `candidate_id`
Fields: `first_name`, `second_name`, `about`

### Matches
Composite key: `position_id`, `candidate_id`
Fields: `unix_timestamp`

### Likes
Composite key: `position_id`, `candidate_id`
Fields: `unix_timestamp`

### Dislikes
Composite key: `position_id`, `candidate_id`
Fields: `unix_timestamp`

## In-memory storage (HashSet)

### Swipes
- On swipe, store key (string) `candidate_id/position_id`.
- If `candidate_id/position_id` indexable in HashSet, write to the `Matches` table.
- Report the (no) match in the response.

## Backend
- `GET` and `CREATE` endpoints for all DB tables.
- `GET` and `CREATE` endpoints for in-memory storage.
- Query param to get unswiped candidate.
- Query param to get random candidate.
- Query param to get unswiped position.
- Query param to get random position.
- Query param to load user swipes (Likes table) into in-memory storage.
## UI

### 1st screen
- Welcome: "Who are you?" -> "Recruiter", "Candidate".
- If "Candidate" -> Show `CandidateView`.
- If "Recruiter" -> Show `RecruiterView`.

### 2nd screen
- On `CandidateView` or `RecruiterView`.
- If clicked `Yes`, validate the match and inform the use, provide next `CandidateCard` or `PositionCard`
- If clicked `No`, just provide next `CandidateCard` or `PositionCard`.
