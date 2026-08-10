# ClawReef Backend

ClawReef virtual desktop management platform backend API.

## Tech Stack

- Golang 1.21+
- Gin 1.9+
- upper/db 4.x
- MySQL 8.0+
- JWT Authentication

## Quick Start

### Prerequisites

- Go 1.21 or higher
- MySQL 8.0+
- Docker (optional)

### Development Setup

1. **Install dependencies**
   ```bash
   make deps
   ```

2. **Start MySQL with Docker**
   ```bash
   make docker-up
   ```

3. **Run database migration**
   ```bash
   make migrate
   ```

4. **Start the server**
   ```bash
   make run
   ```

### API Endpoints

Server runs on port **9001**.

#### Authentication
- `POST /api/v1/auth/register` - User registration
- `POST /api/v1/auth/login` - User login
- `POST /api/v1/auth/refresh` - Refresh token
- `POST /api/v1/auth/logout` - User logout
- `GET /api/v1/auth/me` - Get current user

### Default Admin Account

- Username: `admin`
- Password: `admin123`

## Project Structure

```
backend/
├── cmd/server/          # Application entry point
├── internal/
│   ├── config/          # Configuration
│   ├── db/              # Database connection & migrations
│   ├── models/          # Data models
│   ├── repository/      # Data access layer
│   ├── services/        # Business logic
│   ├── handlers/        # HTTP handlers
│   ├── middleware/      # HTTP middleware
│   └── utils/           # Utilities
├── deployments/         # Docker & K8s configs
└── configs/             # Configuration files
```

## Make Commands

- `make build` - Build the binary
- `make run` - Run the server
- `make test` - Run tests
- `make fmt` - Format code
- `make lint` - Run linter
- `make docker-up` - Start MySQL container
- `make migrate` - Run database migrations
