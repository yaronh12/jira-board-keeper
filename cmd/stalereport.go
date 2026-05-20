package cmd

import (
	"github.com/spf13/cobra"
	"github.com/yaronhod/jira-board-keeper/internal/jira"
	"github.com/yaronhod/jira-board-keeper/internal/slack"
	"github.com/yaronhod/jira-board-keeper/internal/stalereport"
)

var (
	epicThreshold    int
	defaultThreshold int
)

var staleReportCmd = &cobra.Command{
	Use:   "stale-report",
	Short: "Report stale issues to Slack",
	Long:  "Scans board issues for items that haven't had a status change within the configured threshold and reports them to Slack.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !cfg.StaleReport.Enabled && !forceRun {
			logger.Info("stale-report is disabled in config, skipping (use --force to override)")
			return nil
		}

		if err := cfg.Validate(); err != nil {
			return err
		}

		if cmd.Flags().Changed("epic-threshold") {
			cfg.StaleReport.Thresholds["epic"] = epicThreshold
		}
		if cmd.Flags().Changed("default-threshold") {
			for k := range cfg.StaleReport.Thresholds {
				if k != "epic" {
					cfg.StaleReport.Thresholds[k] = defaultThreshold
				}
			}
		}

		jiraClient := jira.NewClient(cfg.Jira.BaseURL, cfg.Jira.Email, cfg.Jira.APIToken, logger)
		slackClient := slack.NewClient(cfg.Slack.WebhookURL, cfg.DryRun, logger)
		reporter := stalereport.New(jiraClient, slackClient, cfg, logger)

		return reporter.Run(cmd.Context())
	},
}

func init() {
	staleReportCmd.Flags().IntVar(&epicThreshold, "epic-threshold", 0, "stale threshold in days for epics (overrides config)")
	staleReportCmd.Flags().IntVar(&defaultThreshold, "default-threshold", 0, "stale threshold in days for non-epic issues (overrides config)")
}
