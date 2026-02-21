# Hirevec Backend

This project implements server for the Hirevec app.

## Setup

We support bare metal setup (downloading postgres and other dependencies is on you) and Docker setup (dependencies are handled automatically).

### Bare Metal

> [!NOTE]
> Bare metal scripts were tested on macOS 15.7.3 and Debian 13.3.

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
4. Open [http://localhost:8080/api/v1/health](http://localhost:8080/api/v1/health).

#### Cleanup

In case, for whatever reason, you want to completely remove the database and all what was created by the setup script, run cleanup script:
```bash
go run cmd/cleanup/main.go
```

### Via Docker

#### Requirements

- docker >= 29.0.1

#### Steps

1. Setup required environment variables in `.env` as shown in [.example.env](.example.env).
2. Run:
```bash
docker compose up
```
3. Open [http://localhost:8080/api/v1/health](http://localhost:8080/api/v1/health).

