## Setup

We support either bare metal setup (downloading postgres and other dependencies is on you) or Docker (dependencies are handled automatically).

### Bare Metal

#### Requirements
- go >= 1.25.5 
- postgres >= 18.1

#### Steps
1. Setup required environment variables in `.env` as shown in [.example.env](.example.env).
2. Run Go setup script:
```bash
go run cmd/setup/main.go
```
3. Run Go server:
```
go run cmd/server/main.go
```

### Via Docker

#### Requirements
- docker >= 29.0.1

#### Steps
1. Setup required environment variables in `.env` as shown in [.example.env](.example.env).
2.  
```bash
docker compose up
```
