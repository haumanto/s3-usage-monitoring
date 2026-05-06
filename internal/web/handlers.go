package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/haumanto/s3-usage-monitoring/internal/db"
	"github.com/haumanto/s3-usage-monitoring/internal/scheduler"
)

var tmpl *template.Template

func InitTemplates(templateDir string) error {
	var err error
	tmpl, err = template.ParseGlob(templateDir + "/*.html")
	return err
}

type AccountView struct {
	db.S3Account
	PercentUsed   int
	UsageGB       float64
	QuotaGB       float64
	FormattedLast string
}

func toAccountView(a db.S3Account) AccountView {
	v := AccountView{S3Account: a}
	if a.QuotaBytes > 0 {
		v.PercentUsed = int(float64(a.CurrentUsageBytes) / float64(a.QuotaBytes) * 100)
		if v.PercentUsed > 100 {
			v.PercentUsed = 100
		}
	}
	v.UsageGB = float64(a.CurrentUsageBytes) / (1024 * 1024 * 1024)
	v.QuotaGB = float64(a.QuotaBytes) / (1024 * 1024 * 1024)
	if a.LastCheckAt != nil {
		v.FormattedLast = a.LastCheckAt.Format("2006-01-02 15:04")
	}
	return v
}

func DashboardHandler(w http.ResponseWriter, r *http.Request) {
	accounts, err := db.GetAllAccounts()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var views []AccountView
	for _, a := range accounts {
		views = append(views, toAccountView(a))
	}

	data := struct {
		Accounts []AccountView
		Now      time.Time
	}{
		Accounts: views,
		Now:      time.Now(),
	}

	w.Header().Set("Content-Type", "text/html")
	if err := tmpl.ExecuteTemplate(w, "dashboard.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]interface{}{"success": false, "error": message})
}

func AccountsHandler(w http.ResponseWriter, r *http.Request) {
	accounts, err := db.GetAllAccounts()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(accounts)
}

func CreateAccountHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	quota, err := scheduler.ParseQuota(r.FormValue("quota"), r.FormValue("quota_unit"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid quota: "+err.Error())
		return
	}

	threshold, _ := strconv.Atoi(r.FormValue("threshold"))
	if threshold <= 0 || threshold > 100 {
		threshold = 80
	}

	account := &db.S3Account{
		Name:             r.FormValue("name"),
		AccessKey:        r.FormValue("access_key"),
		SecretKey:        r.FormValue("secret_key"),
		Region:           r.FormValue("region"),
		Endpoint:         r.FormValue("endpoint"),
		Bucket:           r.FormValue("bucket"),
		QuotaBytes:       quota,
		ThresholdPercent: threshold,
		TelegramEnabled:  r.FormValue("telegram_enabled") == "on",
		TelegramBotToken: r.FormValue("telegram_bot_token"),
		TelegramChatID:   r.FormValue("telegram_chat_id"),
	}

	if err := db.CreateAccount(account); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "account": account})
}

func UpdateAccountHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.URL.Path[len("/api/accounts/"):], 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	if err := r.ParseForm(); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	account, err := db.GetAccount(id)
	if err != nil || account == nil {
		writeJSONError(w, http.StatusNotFound, "Account not found")
		return
	}

	quota, err := scheduler.ParseQuota(r.FormValue("quota"), r.FormValue("quota_unit"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid quota: "+err.Error())
		return
	}

	threshold, _ := strconv.Atoi(r.FormValue("threshold"))
	if threshold <= 0 || threshold > 100 {
		threshold = 80
	}

	account.Name = r.FormValue("name")
	account.AccessKey = r.FormValue("access_key")
	account.SecretKey = r.FormValue("secret_key")
	account.Region = r.FormValue("region")
	account.Endpoint = r.FormValue("endpoint")
	account.Bucket = r.FormValue("bucket")
	account.QuotaBytes = quota
	account.ThresholdPercent = threshold
	account.TelegramEnabled = r.FormValue("telegram_enabled") == "on"
	account.TelegramBotToken = r.FormValue("telegram_bot_token")
	account.TelegramChatID = r.FormValue("telegram_chat_id")

	if err := db.UpdateAccount(account); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "account": account})
}

func DeleteAccountHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.URL.Path[len("/api/accounts/"):], 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	if err := db.DeleteAccount(id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true})
}

func GetAccountHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.URL.Path[len("/api/accounts/"):], 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	account, err := db.GetAccount(id)
	if err != nil || account == nil {
		writeJSONError(w, http.StatusNotFound, "Account not found")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(account)
}

func SettingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		interval, _ := db.GetSetting("check_interval")
		globalBotToken, _ := db.GetSetting("telegram_bot_token")
		globalChatID, _ := db.GetSetting("telegram_chat_id")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"check_interval":     interval,
			"telegram_bot_token": globalBotToken,
			"telegram_chat_id":   globalChatID,
		})
		return
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}

		if interval := r.FormValue("check_interval"); interval != "" {
			if _, err := time.ParseDuration(interval); err != nil {
				writeJSONError(w, http.StatusBadRequest, "Invalid interval")
				return
			}
			_ = db.SetSetting("check_interval", interval)
		}

		_ = db.SetSetting("telegram_bot_token", r.FormValue("telegram_bot_token"))
		_ = db.SetSetting("telegram_chat_id", r.FormValue("telegram_chat_id"))

		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true})
		return
	}

	writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().Unix(),
	})
}

func ConfigPageHandler(w http.ResponseWriter, r *http.Request) {
	accounts, err := db.GetAllAccounts()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	if err := tmpl.ExecuteTemplate(w, "config.html", accounts); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func SettingsPageHandler(w http.ResponseWriter, r *http.Request) {
	interval, _ := db.GetSetting("check_interval")
	globalBotToken, _ := db.GetSetting("telegram_bot_token")
	globalChatID, _ := db.GetSetting("telegram_chat_id")

	data := struct {
		CheckInterval    string
		TelegramBotToken string
		TelegramChatID   string
	}{
		CheckInterval:    interval,
		TelegramBotToken: globalBotToken,
		TelegramChatID:   globalChatID,
	}

	w.Header().Set("Content-Type", "text/html")
	if err := tmpl.ExecuteTemplate(w, "settings.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func TriggerCheckHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Path[len("/api/trigger/"):]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	account, err := db.GetAccount(id)
	if err != nil || account == nil {
		writeJSONError(w, http.StatusNotFound, "Account not found")
		return
	}

	// Run check in background
	go func() {
		if err := scheduler.CheckAccount(account); err != nil {
			fmt.Printf("Manual check failed for %s: %v\n", account.Name, err)
		}
	}()

	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "Check triggered"})
}
