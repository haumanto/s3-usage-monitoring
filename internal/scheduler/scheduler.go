package scheduler

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/haumanto/s3-usage-monitoring/internal/db"
	"github.com/haumanto/s3-usage-monitoring/internal/s3"
	"github.com/haumanto/s3-usage-monitoring/internal/telegram"
)

type Scheduler struct {
	ticker   *time.Ticker
	stop     chan struct{}
	interval time.Duration
	mu       sync.Mutex
	running  bool
}

func New() *Scheduler {
	return &Scheduler{
		stop: make(chan struct{}),
	}
}

func (s *Scheduler) Start() error {
	interval, err := s.loadInterval()
	if err != nil {
		return err
	}
	s.interval = interval
	s.ticker = time.NewTicker(interval)

	// Run immediately on start
	go s.runCheck()

	go func() {
		for {
			select {
			case <-s.ticker.C:
				s.runCheck()
			case <-s.stop:
				s.ticker.Stop()
				return
			}
		}
	}()

	return nil
}

func (s *Scheduler) Stop() {
	close(s.stop)
}

func (s *Scheduler) UpdateInterval() {
	interval, err := s.loadInterval()
	if err != nil {
		log.Printf("Failed to load interval: %v", err)
		return
	}
	if interval != s.interval {
		s.interval = interval
		s.ticker.Reset(interval)
		log.Printf("Scheduler interval updated to %v", interval)
	}
}

func (s *Scheduler) loadInterval() (time.Duration, error) {
	val, err := db.GetSetting("check_interval")
	if err != nil {
		return 5 * time.Minute, err
	}
	if val == "" {
		return 5 * time.Minute, nil
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return 5 * time.Minute, err
	}
	if d < time.Minute {
		return time.Minute, nil
	}
	return d, nil
}

func (s *Scheduler) runCheck() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		log.Printf("Previous check still running, skipping")
		return
	}
	s.running = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	accounts, err := db.GetAllAccounts()
	if err != nil {
		log.Printf("Failed to get accounts: %v", err)
		return
	}

	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup

	for _, account := range accounts {
		wg.Add(1)
		go func(acc db.S3Account) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := CheckAccount(&acc); err != nil {
				log.Printf("Check failed for %s: %v", acc.Name, err)
			}
		}(account)
	}

	wg.Wait()
}

func resolveTelegramCreds(account *db.S3Account) (token, chatID string) {
	token = account.TelegramBotToken
	chatID = account.TelegramChatID
	if token == "" {
		token, _ = db.GetSetting("telegram_bot_token")
	}
	if chatID == "" {
		chatID, _ = db.GetSetting("telegram_chat_id")
	}
	return token, chatID
}

func CheckAccount(account *db.S3Account) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	oldStatus := account.LastCheckStatus

	client, err := s3.NewClient(account)
	if err != nil {
		if updErr := db.UpdateAccountUsage(account.ID, account.CurrentUsageBytes, "error", err.Error()); updErr != nil {
			log.Printf("Failed to update account usage for %s: %v", account.Name, updErr)
		}
		return err
	}

	usage, err := client.CalculateUsage(ctx)
	if err != nil {
		if updErr := db.UpdateAccountUsage(account.ID, account.CurrentUsageBytes, "error", err.Error()); updErr != nil {
			log.Printf("Failed to update account usage for %s: %v", account.Name, updErr)
		}

		// Send error notification if telegram is enabled
		token, chatID := resolveTelegramCreds(account)
		if account.TelegramEnabled && token != "" && chatID != "" {
			bot := telegram.NewBot(token, chatID)
			if sendErr := bot.SendErrorNotification(account.Name, err.Error()); sendErr != nil {
				log.Printf("Failed to send error notification for %s: %v", account.Name, sendErr)
			}
		}
		return err
	}

	var newStatus string

	if account.QuotaBytes > 0 {
		percentUsed := float64(usage) / float64(account.QuotaBytes) * 100
		if percentUsed >= float64(account.ThresholdPercent) {
			newStatus = "warning"
		} else {
			newStatus = "ok"
		}
	} else {
		newStatus = "ok"
	}

	if err := db.UpdateAccountUsage(account.ID, usage, newStatus, ""); err != nil {
		return fmt.Errorf("update usage: %w", err)
	}

	// Only send Telegram notifications when status changes
	token, chatID := resolveTelegramCreds(account)
	if account.TelegramEnabled && token != "" && chatID != "" {
		if oldStatus != newStatus {
			bot := telegram.NewBot(token, chatID)
			switch {
			case newStatus != "ok":
				// warning, critical, or any other non-ok status
				if sendErr := bot.SendNotification(account.Name, usage, account.QuotaBytes, account.ThresholdPercent); sendErr != nil {
					log.Printf("Failed to send threshold notification for %s: %v", account.Name, sendErr)
				}
			case newStatus == "ok" && oldStatus != "" && oldStatus != "ok":
				// recovered from warning/critical/error to ok
				if sendErr := bot.SendRecoveryNotification(account.Name, usage, account.QuotaBytes); sendErr != nil {
					log.Printf("Failed to send recovery notification for %s: %v", account.Name, sendErr)
				}
			}
		} else {
			log.Printf("Skipping Telegram notification for %s: status unchanged (%s)", account.Name, newStatus)
		}
	}

	log.Printf("Account %s: usage=%d bytes, oldStatus=%s, newStatus=%s", account.Name, usage, oldStatus, newStatus)
	return nil
}

func ParseQuota(quotaStr string, unit string) (int64, error) {
	if quotaStr == "" {
		quotaStr = "10"
	}
	val, err := strconv.ParseFloat(quotaStr, 64)
	if err != nil {
		return 0, err
	}
	switch unit {
	case "GB":
		return int64(val * 1024 * 1024 * 1024), nil
	case "TB":
		return int64(val * 1024 * 1024 * 1024 * 1024), nil
	default:
		return int64(val * 1024 * 1024 * 1024), nil
	}
}
