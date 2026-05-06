# S3 Usage Monitoring Dashboard

A lightweight, self-hosted Go web application for monitoring S3 storage usage across multiple accounts with quota tracking and Telegram notifications.

## Features

- **Multi-account S3 support** — Configure one or more S3 accounts (AWS, MinIO, Wasabi, etc.)
- **Quota management** — Set storage quotas per account with visual progress bars
- **Threshold alerts** — Configurable usage threshold per account (default 80%)
- **Telegram notifications** — Per-account Telegram bot alerts when thresholds are crossed
- **Automatic polling** — Background scheduler checks usage on a configurable interval
- **Clean web dashboard** — Dark-themed UI with status indicators and real-time data
- **SQLite database** — Embedded, zero-config database
- **Docker ready** — Single container with health checks

## Screenshots

- **Dashboard** — Shows all accounts with usage bars, quota info, and status (OK / Warning / Error)
- **Accounts** — Add, edit, and delete S3 account configurations
- **Settings** — Configure global check interval and default Telegram credentials

## Quick Start (Docker Compose)

```bash
git clone https://github.com/haumanto/s3-usage-monitoring.git
cd s3-usage-monitoring
docker compose up --build
```

Open http://localhost:8080 in your browser.

## Configuration

### Environment Variables

| Variable       | Default                | Description                          |
|----------------|------------------------|--------------------------------------|
| `PORT`         | `8080`                 | HTTP server port                     |
| `DB_PATH`      | `./data/s3monitor.db`  | SQLite database file path            |
| `TEMPLATE_DIR` | `./web/templates`      | HTML templates directory             |
| `STATIC_DIR`   | `./web/static`         | Static assets directory              |

### S3 Account Setup

1. Go to **Accounts** and click **Add Account**
2. Fill in:
   - **Account Name** — A friendly name for this account
   - **Access Key ID / Secret Access Key** — Your S3 credentials
   - **Region** — e.g. `us-east-1` (default), `eu-west-1`
   - **Custom Endpoint** — For S3-compatible stores (MinIO, Wasabi, etc.)
   - **Bucket** — Optional. Leave empty to sum usage across all buckets
   - **Quota** — Storage limit (GB or TB)
   - **Threshold %** — When to trigger a warning (default 80%)
3. Optionally enable **Telegram notifications** and provide bot token + chat ID

### Getting AWS Credentials

1. Log in to the [AWS IAM Console](https://console.aws.amazon.com/iam/)
2. Create a user with programmatic access
3. Attach a policy with `s3:ListAllMyBuckets`, `s3:ListBucket`, and `s3:GetObject` permissions
4. Copy the **Access Key ID** and **Secret Access Key**

### Getting a Telegram Bot Token

1. Message [@BotFather](https://t.me/BotFather) on Telegram
2. Create a new bot with `/newbot`
3. Copy the provided bot token
4. To get your **Chat ID**, message [@userinfobot](https://t.me/userinfobot) or use the Bot API:  
   `https://api.telegram.org/bot<TOKEN>/getUpdates`

### Usage Calculation Approach

The app uses **ListObjectsV2** to iterate objects and sum their sizes. This approach:
- Works with any S3-compatible API
- Does not require CloudWatch or billing access
- Is accurate for object-level usage

> **Note:** For buckets with millions of objects, the first check may take a while. Subsequent checks benefit from the same API.

## Development

### Requirements

- Go 1.23+
- GCC (for SQLite CGO bindings)

### Build locally

```bash
go mod tidy
go build -o s3-monitor ./cmd/server
./s3-monitor
```

### Project Structure

```
s3-usage-monitoring/
├── cmd/server/main.go        # Entry point
├── internal/
│   ├── db/                   # SQLite models and queries
│   ├── s3/                   # AWS SDK v2 S3 client
│   ├── scheduler/            # Background polling scheduler
│   ├── telegram/             # Telegram Bot API client
│   └── web/                  # HTTP handlers and routing
├── web/
│   ├── templates/            # Go HTML templates
│   └── static/               # CSS and assets
├── Dockerfile
├── docker-compose.yml
└── README.md
```

## API Endpoints

| Endpoint                | Method | Description                     |
|-------------------------|--------|---------------------------------|
| `/`                     | GET    | Dashboard page                  |
| `/config`               | GET    | Accounts configuration page     |
| `/settings`             | GET    | Settings page                   |
| `/health`               | GET    | Health check (JSON)             |
| `/api/accounts`         | GET    | List all accounts (JSON)        |
| `/api/accounts`         | POST   | Create account                  |
| `/api/accounts/{id}`    | GET    | Get single account              |
| `/api/accounts/{id}`    | POST   | Update account                  |
| `/api/accounts/{id}`    | DELETE | Delete account                  |
| `/api/settings`         | GET    | Get global settings             |
| `/api/settings`         | POST   | Update global settings          |
| `/api/trigger/{id}`     | POST   | Trigger manual check for account|

## License

MIT
