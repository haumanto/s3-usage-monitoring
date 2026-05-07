package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Bot struct {
	BotToken string
	ChatID   string
	client   *http.Client
}

func NewBot(token, chatID string) *Bot {
	return &Bot{
		BotToken: token,
		ChatID:   chatID,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (b *Bot) IsConfigured() bool {
	return b.BotToken != "" && b.ChatID != ""
}

func (b *Bot) SendNotification(accountName string, usageBytes, quotaBytes int64, thresholdPercent int) error {
	if !b.IsConfigured() {
		return fmt.Errorf("telegram bot not configured")
	}

	usageGB := float64(usageBytes) / (1024 * 1024 * 1024)
	quotaGB := float64(quotaBytes) / (1024 * 1024 * 1024)
	percentUsed := 0.0
	if quotaBytes > 0 {
		percentUsed = float64(usageBytes) / float64(quotaBytes) * 100
	}

	message := fmt.Sprintf(
		"🚨 *S3 Usage Alert*\n\n"+
			"*Account:* %s\n"+
			"*Usage:* %.2f GB / %.2f GB\n"+
			"*Percent Used:* %.1f%%\n"+
			"*Threshold:* %d%%\n\n"+
			"⚠️ Usage has exceeded the configured threshold!",
		accountName, usageGB, quotaGB, percentUsed, thresholdPercent,
	)

	return b.sendMessage(message)
}

func (b *Bot) SendRecoveryNotification(accountName string, usageBytes, quotaBytes int64) error {
	if !b.IsConfigured() {
		return fmt.Errorf("telegram bot not configured")
	}

	usageGB := float64(usageBytes) / (1024 * 1024 * 1024)
	quotaGB := float64(quotaBytes) / (1024 * 1024 * 1024)
	percentUsed := 0.0
	if quotaBytes > 0 {
		percentUsed = float64(usageBytes) / float64(quotaBytes) * 100
	}

	message := fmt.Sprintf(
		"✅ *S3 Usage Recovered*\n\n"+
			"*Account:* %s\n"+
			"*Usage:* %.2f GB / %.2f GB\n"+
			"*Percent Used:* %.1f%%\n\n"+
			"Usage is now back within normal limits.",
		accountName, usageGB, quotaGB, percentUsed,
	)

	return b.sendMessage(message)
}

func (b *Bot) SendErrorNotification(accountName string, errMsg string) error {
	if !b.IsConfigured() {
		return fmt.Errorf("telegram bot not configured")
	}

	message := fmt.Sprintf(
		"❌ *S3 Check Error*\n\n"+
			"*Account:* %s\n"+
			"*Error:* %s",
		accountName, errMsg,
	)

	return b.sendMessage(message)
}

func (b *Bot) sendMessage(text string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", b.BotToken)

	payload := map[string]interface{}{
		"chat_id":    b.ChatID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := b.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned status %d", resp.StatusCode)
	}

	var result struct {
		OK bool `json:"ok"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if !result.OK {
		return fmt.Errorf("telegram API returned ok=false")
	}

	return nil
}