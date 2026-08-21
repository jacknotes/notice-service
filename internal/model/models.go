package model

import "time"

type User struct {
	ID           int64      `json:"id"`
	Username     string     `json:"username"`
	DisplayName  string     `json:"display_name"`
	Email        string     `json:"email"`
	PasswordHash string     `json:"-"`
	Role         string     `json:"role"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    *time.Time `json:"-"`

	// 双因子认证（TOTP）
	TOTPSecret         string   `json:"-"`
	TOTPEnabled        bool     `json:"totp_enabled"`
	TOTPRecoveryCodes  []string `json:"-"`
	TOTPRecoveryJSON   string   `json:"-"`
}

type Channel struct {
	ID         int64             `json:"id"`
	UserID     int64             `json:"user_id"`
	Type       string            `json:"type"`
	Name       string            `json:"name"`
	Config     map[string]string `json:"config"`
	ConfigJSON string            `json:"-"`
	Enabled    bool              `json:"enabled"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
	DeletedAt  *time.Time        `json:"-"`
}

type TemplateVar struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Default     string `json:"default"`
}

type Template struct {
	ID            int64         `json:"id"`
	UserID        int64         `json:"user_id"`
	Name          string        `json:"name"`
	Subject       string        `json:"subject"`
	ContentMD     string        `json:"content_md"`
	Variables     []TemplateVar `json:"variables"`
	VariablesJSON string        `json:"-"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
	DeletedAt     *time.Time    `json:"-"`
}

type Task struct {
	ID             int64             `json:"id"`
	UserID         int64             `json:"user_id"`
	Name           string            `json:"name"`
	ChannelID      int64             `json:"channel_id"`
	ChannelIDs     []int64           `json:"channel_ids,omitempty"`
	ChannelIDsJSON string            `json:"-"`
	TemplateID     int64             `json:"template_id"`
	TriggerType    string            `json:"trigger_type"`
	Receivers      []string          `json:"receivers"`
	ReceiversJSON  string            `json:"-"`
	CronExpr       string            `json:"cron_expr"`
	APIKey         string            `json:"api_key,omitempty"`
	AllowedIPs     []string          `json:"allowed_ips,omitempty"`
	AllowedIPsJSON string            `json:"-"`
	Variables      map[string]string `json:"variables,omitempty"`
	VariablesJSON  string            `json:"-"`
	LockedBy       string            `json:"-"`
	LockedAt       *time.Time        `json:"-"`
	Enabled        bool              `json:"enabled"`
	LastRunAt      *time.Time        `json:"last_run_at"`
	NextRunAt      *time.Time        `json:"next_run_at"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	DeletedAt      *time.Time        `json:"-"`
}

type TaskLog struct {
	ID         int64     `json:"id"`
	TaskID     int64     `json:"task_id"`
	ChannelID  int64     `json:"channel_id"`
	Subject    string    `json:"subject"`
	Content    string    `json:"content"`
	Status     string    `json:"status"`
	Request    string    `json:"request"`
	Response   string    `json:"response"`
	ErrorMsg   string    `json:"error_msg"`
	RetryCount int       `json:"retry_count"`
	TriggerType string   `json:"trigger_type"`
	TriggerBy  string    `json:"trigger_by"`
	TriggerIP  string    `json:"trigger_ip"`
	SentAt     time.Time `json:"sent_at"`
}

type SendJob struct {
	ID          int64      `json:"id"`
	TaskID      int64      `json:"task_id"`
	LogID       int64      `json:"-"`
	TriggerType string     `json:"-"`
	TriggerBy   string     `json:"-"`
	TriggerIP   string     `json:"-"`
	VarsJSON    string     `json:"-"`
	Status      string     `json:"status"`
	ClaimedBy   string     `json:"-"`
	ClaimedAt   *time.Time `json:"-"`
	Attempts    int        `json:"attempts"`
	NextRetryAt *time.Time `json:"-"`
	LastError   string     `json:"-"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	SentAt      *time.Time `json:"-"`
	DedupeKey   string     `json:"-"`
}
