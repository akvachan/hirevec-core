## Run Server
- Setup environment variables, refer to [this](#environment-variables) section.
- Setup development PGSQL database, refer to [this](#setup-pgsql) section.
- Run server with: 
```
go run cmd/server
```

## Unit Tests
- Run tests with:
```
go test internal -v
```

## Environment Variables
- `APP_HOST`: Host for the server (default: `localhost`)
- `APP_PORT`: Port for the server (default: `8080`)
- `REQUEST_READ_TIMEOUT`: Request read timeout duration (default: `2000ms`)
- `REQUEST_WRITE_TIMEOUT`: Request write timeout duration (default: `2000ms`)
- `GRACE_PERIOD`: Grace period for server shutdown (default: `5000ms`)
- `PGHOST`: PostgreSQL host (default: `localhost`)
- `PGPORT`: PostgreSQL port (default: `5432`)
- `PGDATABASE`: PostgreSQL database name (default: `hirevec`)
- `PGUSER`: PostgreSQL user (create your own)
- `PGPASSWORD`: PostgreSQL password (create your own)
- `REDIS_URL`: Redis connection URL (default: `redis://localhost:6379/0`)
- `LOG_LEVEL`: Log level (default: `ERROR`)
- `SYMMETRIC_KEY`: Symmetric key for refresh tokens (create your own, must be 32 bytes)
- `ASYMMETRIC_KEY`: Asymmetric key for access tokens (create your own, must be 64 bytes)
