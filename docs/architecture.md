## DB tables

### Users
| Column     | Type         | Constraints      |
| ---------- | ------------ | ---------------- |
| id         | INT          | PK               |
| email      | VARCHAR(512) | NOT NULL, UNIQUE |
| user\_name | VARCHAR(64)  | NOT NULL, UNIQUE |
| full\_name | VARCHAR(128) | NOT NULL         |

### Candidates
| Column   | Type | Constraints                                  |
| -------- | ---- | -------------------------------------------- |
| id       | INT  | PK                                           |
| user\_id | INT  | NOT NULL, FK, ON DELETE CASCADE              |
| about    | TEXT | NOT NULL                                     |

### Recruiters
| Column   | Type | Constraints                                  |
| -------- | ---- | -------------------------------------------- |
| id       | INT  | PK                                           |
| user\_id | INT  | NOT NULL, FK, ON DELETE CASCADE              |

### Positions
| Column      | Type  | Constraints |
|-------------|------ |-------------|
| id          | INT   | PK          |
| title       | TEXT  | NOT NULL    |
| description | TEXT  | NOT NULL    |
| company     | TEXT  |             |

## Candidates' reactions
| Column         | Type                | Constraints                |
| -------------  | ------------------- | -------------------------- |
| candidate\_id  | INT                 | PK, FK, ON DELETE CASCADE  |
| position\_id   | INT                 | PK, FK, ON DELETE CASCADE  |
| reaction\_type | ENUM(like, dislike) | NOT NULL                   |
| created\_at    | TIMESTAMP           | NOT NULL, DEFAULT `NOW()`  |

## Recrutiers' reactions
| Column         | Type                | Constraints                |
| -------------  | ------------------- | -------------------------- |
| recruiter\_id  | INT                 | PK, FK, ON DELETE CASCADE  |
| position\_id   | INT                 | PK, FK, ON DELETE CASCADE  |
| candidate\_id  | INT                 | PK, FK, ON DELETE CASCADE  |
| reaction\_type | ENUM(like, dislike) | NOT NULL                   |
| created\_at    | TIMESTAMP           | NOT NULL, DEFAULT `NOW()`  |

## Matches
| Column        | Type      | Constraints               |
| ------------  | --------- | ------------------------- |
| candidate\_id | INT       | FK, ON DELETE CASCADE     |
| position\_id  | INT       | FK, ON DELETE CASCADE     |
| timestamp     | TIMESTAMP | NOT NULL, DEFAULT `NOW()` |


```mermaid
---
config:
  layout: elk
---
erDiagram

    USERS {
        int id PK
        varchar email
        varchar user_name
        varchar full_name
    }

    CANDIDATES {
        int id PK
        int user_id FK
        text about
    }

    RECRUITERS {
        int id PK
        int user_id FK
    }

    POSITIONS {
        int id PK
        text company
        text description
    }

    CANDIDATES_REACTIONS {
        int candidate_id PK, FK
        int position_id PK, FK
        enum reaction_type
        timestamp created_at
    }

    RECRUITERS_REACTIONS {
        int recruiter_id PK, FK
        int position_id PK, FK
        int candidate_id PK, FK
        enum reaction_type
        timestamp created_at
    }

    MATCHES {
        int id PK
        int candidate_id FK
        int position_id FK
        timestamp timestamp
    }
    USERS ||--o| CANDIDATES : "has"
    USERS ||--o| RECRUITERS : "has"

    CANDIDATES ||--o{ CANDIDATES_REACTIONS : "reacts"
    POSITIONS  ||--o{ CANDIDATES_REACTIONS : "receives"

    RECRUITERS ||--o{ RECRUITERS_REACTIONS : "reacts"
    POSITIONS  ||--o{ RECRUITERS_REACTIONS : "context"
    CANDIDATES ||--o{ RECRUITERS_REACTIONS : "target"

    CANDIDATES ||--o{ MATCHES : "matched"
    POSITIONS  ||--o{ MATCHES : "matched"
```
