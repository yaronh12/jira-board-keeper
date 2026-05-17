package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Jira            JiraConfig        `mapstructure:"jira"`
	Team            TeamConfig        `mapstructure:"team"`
	Board           BoardConfig        `mapstructure:"board"`
	Slack           SlackConfig        `mapstructure:"slack"`
	StaleThresholds map[string]int     `mapstructure:"stale_thresholds"`
	LabelSync       LabelSyncConfig    `mapstructure:"label_sync"`
	StatusReport    StatusReportConfig `mapstructure:"status_report"`
	LogLevel        string            `mapstructure:"log_level"`
	DryRun          bool              `mapstructure:"dry_run"`
}

type JiraConfig struct {
	BaseURL  string `mapstructure:"base_url"`
	Email    string `mapstructure:"-"`
	APIToken string `mapstructure:"-"`
}

type TeamConfig struct {
	Name    string   `mapstructure:"name"`
	Members []string `mapstructure:"members"`
}

type BoardConfig struct {
	Label       string   `mapstructure:"label"`
	ProjectKeys []string `mapstructure:"project_keys"`
	JQLFilter   string   `mapstructure:"jql_filter"`
}

type SlackConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	WebhookURL string `mapstructure:"-"`
}

type LabelSyncConfig struct {
	LookbackDays int `mapstructure:"lookback_days"`
}

type StatusReportConfig struct {
	LookbackDays int `mapstructure:"lookback_days"`
}

func Load(path string) (*Config, error) {
	v := viper.New()

	v.SetDefault("log_level", "info")
	v.SetDefault("dry_run", false)
	v.SetDefault("slack.enabled", true)
	v.SetDefault("label_sync.lookback_days", 7)
	v.SetDefault("status_report.lookback_days", 7)
	v.SetDefault("stale_thresholds", map[string]int{
		"epic":    60,
		"story":   30,
		"task":    30,
		"bug":     30,
		"sub_task": 30,
		"default": 30,
	})

	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	v.SetEnvPrefix("JBR")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	cfg.Jira.Email = getEnv("JIRA_EMAIL", "")
	cfg.Jira.APIToken = getEnv("JIRA_API_TOKEN", "")
	cfg.Slack.WebhookURL = getEnv("SLACK_WEBHOOK_URL", "")

	if v.IsSet("dry_run") {
		cfg.DryRun = v.GetBool("dry_run")
	}
	if envDry := os.Getenv("JBR_DRY_RUN"); envDry != "" {
		cfg.DryRun = envDry == "true" || envDry == "1"
	}

	return &cfg, nil
}

func (c *Config) Validate() error {
	if c.Jira.BaseURL == "" {
		return fmt.Errorf("jira.base_url is required")
	}
	if len(c.Team.Members) == 0 {
		return fmt.Errorf("team.members must have at least one entry")
	}
	if c.Board.Label == "" {
		return fmt.Errorf("board.label is required")
	}
	if c.Jira.Email == "" || c.Jira.APIToken == "" {
		return fmt.Errorf("JIRA_EMAIL and JIRA_API_TOKEN environment variables are required")
	}
	if c.Slack.Enabled && c.Slack.WebhookURL == "" {
		return fmt.Errorf("SLACK_WEBHOOK_URL environment variable is required when slack is enabled")
	}
	return nil
}

func (c *Config) GetStaleThreshold(issueType string) int {
	normalized := strings.ToLower(strings.ReplaceAll(issueType, "-", "_"))
	normalized = strings.ReplaceAll(normalized, " ", "_")
	if days, ok := c.StaleThresholds[normalized]; ok {
		return days
	}
	if days, ok := c.StaleThresholds["default"]; ok {
		return days
	}
	return 30
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
