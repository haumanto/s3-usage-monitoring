package scheduler

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/haumanto/s3-usage-monitoring/internal/db"
	"github.com/haumanto/s3-usage-monitoring/internal/s3"
	"github.com/haumanto/s3-usage-monitoring/internal/telegram"
)

type Scheduler struct {
	ticker   *time.Ticker
	stop     chan struct{}
	interval time.Duration
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
	accounts, err := db.GetAllAccounts()
	if err != nil {
		log.Printf("Failed to get accounts: %v", err)
		return
	}

	for _, account := range accounts {
		go func(acc db.S3Account) {
			if err := CheckAccount(&acc); err != nil {
				log.Printf("Check failed for %s: %v", acc.Name, err)
			}
		}(account)
	}
}

func CheckAccount(account *db.S3Account) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client, err := s3.NewClient(account)
	if err != nil {
		_ = db.UpdateAccountUsage(account.ID, account.CurrentUsageBytes, "error", err.Error())
		return err
	}

	usage, err := client.CalculateUsage(ctx)
	if err != nil {
		_ = db.UpdateAccountUsage(account.ID, account.CurrentUsageBytes, "error", err.Error())

		// Send error notification if telegram is enabled
		if account.TelegramEnabled && account.TelegramBotToken != "" && account.TelegramChatID != "" {
			bot := telegram.NewBot(account.TelegramBotToken, account.TelegramChatID)
			_ = bot.SendErrorNotification(account.Name, err.Error())
		}
		return err
	}

	var status string
	var thresholdCrossed bool

	if account.QuotaBytes > 0 {
		percentUsed := float64(usage) / float64(account.QuotaBytes) * 100
		if percentUsed >= float64(account.ThresholdPercent) {
			status = "warning"
			thresholdCrossed = true
		} else {
			status = "ok"
		}
	} else {
		status = "ok"
	}

	if err := db.UpdateAccountUsage(account.ID, usage, status, ""); err != nil {
		return fmt.Errorf("update usage: %w", err)
	}

	// Send threshold notification
	if thresholdCrossed && account.TelegramEnabled && account.TelegramBotToken != "" && account.TelegramChatID != "" {
		bot := telegram.NewBot(account.TelegramBotToken, account.TelegramChatID)
		_ = bot.SendNotification(account.Name, usage, account.QuotaBytes, account.ThresholdPercent)
	}

	log.Printf("Account %s: usage=%d bytes, status=%s", account.Name, usage, status)
	return nil
}

func ParseQuota(quotaStr string, unit string) (int64, error) {
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
