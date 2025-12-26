# hirevec-backend

> Find your perfect job blazingly fast!

## Features
- Smooth registration 
	- Policy: App **can** only ask user to fill out up to 5 fields before giving user access to the main view
	- Feature: Upload CV or register via LinkedIn, Google or Apple OAuth
- Simple UI
    - No learning curve
    - Minimal UI design
    - Single view/screen
- Offline access
	- In a train
	- Areas with network or electricity outages
    - Anywhere!
- No ads 
- Free and open source (MIT license)
    - Healthy concurrents are welcome!
    - Open source contributions are welcome!
    - Deployment on company servers possible and free of charge
- Strong anti-spam
	- Both on the side of candidate and recruiter
    - Anti-spam for matching, chatting and posting
- Ability to chat directly after a match
    - Chat with email notifications
    - Peer-to-peer encrypted
- High performance
	- App will always perform better than alternatives
    - Baseline: 1000 frontend swipes per second, <5ms match detection, <30ms UI click feedback
- Generous premium package
    - 1000 swipes per day
    - Ability to see a potential match
    - Best positions served first
    - Simple filtering system
    - Ability to choose a visual theme / appearance
- Privacy and security
    - Two-factor authentication if registered via CV or OAuth
    - Ability to hard delete **all** data in a single click 
    - EU-level GDPR compliance for **all** users
    - Anonymized company and candidate profiles possible
    - No soft deletes
    - No read / sent / other types of ping feedback in chat
    - No ability to lookup profiles, candidates or positions manually

## Tasks tracking system
In order to create a task:
1. Go to a place in code where something needs to be done and type: `\\ TODO ./todos/DDMMYY-hhmmss-Title.md`.
2. `cp` from `./todos/templates/Todo.md` to `./todos/DDMMYY-hhmmss-Title.md`
3. `open ./todos/DDMMYY-hhmmss-Title.md`
4. Fill out info in `{{}}`
5. After task is done, set `status: Done` and remove the comment from the code.

## Installation
- Development setup with a hot-reload:
```
make watch
```

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
