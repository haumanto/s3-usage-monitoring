package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

var DB *sql.DB

type S3Account struct {
	ID                int64
	Name              string
	AccessKey         string
	SecretKey         string
	Region            string
	Endpoint          string
	Bucket            string
	QuotaBytes        int64
	ThresholdPercent  int
	TelegramEnabled   bool
	TelegramBotToken  string
	TelegramChatID    string
	CurrentUsageBytes int64
	LastCheckAt       *time.Time
	LastCheckStatus   string
	LastCheckError    string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func Init() error {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./data/s3monitor.db"
	}

	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	var err error
	DB, err = sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}

	if err := migrate(); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	return nil
}

func migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS s3_accounts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	access_key TEXT NOT NULL,
	secret_key TEXT NOT NULL,
	region TEXT NOT NULL DEFAULT 'us-east-1',
	endpoint TEXT,
	bucket TEXT,
	quota_bytes INTEGER NOT NULL DEFAULT 10737418240,
	threshold_percent INTEGER NOT NULL DEFAULT 80,
	telegram_enabled INTEGER NOT NULL DEFAULT 0,
	telegram_bot_token TEXT,
	telegram_chat_id TEXT,
	current_usage_bytes INTEGER NOT NULL DEFAULT 0,
	last_check_at DATETIME,
	last_check_status TEXT NOT NULL DEFAULT 'ok',
	last_check_error TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS settings (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

INSERT OR IGNORE INTO settings (key, value) VALUES ('check_interval', '5m');
INSERT OR IGNORE INTO settings (key, value) VALUES ('telegram_bot_token', '');
INSERT OR IGNORE INTO settings (key, value) VALUES ('telegram_chat_id', '');
`

	_, err := DB.Exec(schema)
	return err
}

func GetAllAccounts() ([]S3Account, error) {
	rows, err := DB.Query(`
		SELECT id, name, access_key, secret_key, region, endpoint, bucket,
			quota_bytes, threshold_percent, telegram_enabled, telegram_bot_token, telegram_chat_id,
			current_usage_bytes, last_check_at, last_check_status, last_check_error,
			created_at, updated_at
		FROM s3_accounts ORDER BY created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []S3Account
	for rows.Next() {
		var a S3Account
		var lastCheckAt sql.NullTime
		var lastCheckError sql.NullString
		err := rows.Scan(
			&a.ID, &a.Name, &a.AccessKey, &a.SecretKey, &a.Region, &a.Endpoint, &a.Bucket,
			&a.QuotaBytes, &a.ThresholdPercent, &a.TelegramEnabled, &a.TelegramBotToken, &a.TelegramChatID,
			&a.CurrentUsageBytes, &lastCheckAt, &a.LastCheckStatus, &lastCheckError,
			&a.CreatedAt, &a.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		if lastCheckAt.Valid {
			a.LastCheckAt = &lastCheckAt.Time
		}
		if lastCheckError.Valid {
			a.LastCheckError = lastCheckError.String
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

func GetAccount(id int64) (*S3Account, error) {
	var a S3Account
	var lastCheckAt sql.NullTime
	var lastCheckError sql.NullString
	err := DB.QueryRow(`
		SELECT id, name, access_key, secret_key, region, endpoint, bucket,
			quota_bytes, threshold_percent, telegram_enabled, telegram_bot_token, telegram_chat_id,
			current_usage_bytes, last_check_at, last_check_status, last_check_error,
			created_at, updated_at
		FROM s3_accounts WHERE id = ?
	`, id).Scan(
		&a.ID, &a.Name, &a.AccessKey, &a.SecretKey, &a.Region, &a.Endpoint, &a.Bucket,
		&a.QuotaBytes, &a.ThresholdPercent, &a.TelegramEnabled, &a.TelegramBotToken, &a.TelegramChatID,
		&a.CurrentUsageBytes, &lastCheckAt, &a.LastCheckStatus, &lastCheckError,
		&a.CreatedAt, &a.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if lastCheckAt.Valid {
		a.LastCheckAt = &lastCheckAt.Time
	}
	if lastCheckError.Valid {
		a.LastCheckError = lastCheckError.String
	}
	return &a, err
}

func CreateAccount(a *S3Account) error {
	res, err := DB.Exec(`
		INSERT INTO s3_accounts (name, access_key, secret_key, region, endpoint, bucket,
			quota_bytes, threshold_percent, telegram_enabled, telegram_bot_token, telegram_chat_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, a.Name, a.AccessKey, a.SecretKey, a.Region, a.Endpoint, a.Bucket,
		a.QuotaBytes, a.ThresholdPercent, a.TelegramEnabled, a.TelegramBotToken, a.TelegramChatID)
	if err != nil {
		return err
	}
	a.ID, _ = res.LastInsertId()
	return nil
}

func UpdateAccount(a *S3Account) error {
	_, err := DB.Exec(`
		UPDATE s3_accounts SET
			name = ?, access_key = ?, secret_key = ?, region = ?, endpoint = ?, bucket = ?,
			quota_bytes = ?, threshold_percent = ?, telegram_enabled = ?, telegram_bot_token = ?, telegram_chat_id = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, a.Name, a.AccessKey, a.SecretKey, a.Region, a.Endpoint, a.Bucket,
		a.QuotaBytes, a.ThresholdPercent, a.TelegramEnabled, a.TelegramBotToken, a.TelegramChatID,
		a.ID)
	return err
}

func DeleteAccount(id int64) error {
	_, err := DB.Exec(`DELETE FROM s3_accounts WHERE id = ?`, id)
	return err
}

func UpdateAccountUsage(id int64, usage int64, status string, errMsg string) error {
	_, err := DB.Exec(`
		UPDATE s3_accounts SET
			current_usage_bytes = ?,
			last_check_at = CURRENT_TIMESTAMP,
			last_check_status = ?,
			last_check_error = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, usage, status, errMsg, id)
	return err
}

func GetSetting(key string) (string, error) {
	var val string
	err := DB.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return val, err
}

func SetSetting(key, value string) error {
	_, err := DB.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}
