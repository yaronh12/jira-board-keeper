package cmd

import (
	"github.com/spf13/cobra"
	"github.com/yaronhod/jira-board-keeper/internal/jira"
	"github.com/yaronhod/jira-board-keeper/internal/slack"
	"github.com/yaronhod/jira-board-keeper/internal/statusreport"
)

var lookbackDays int

var statusReportCmd = &cobra.Command{
	Use:   "status-report",
	Short: "Send weekly status report to Slack",
	Long:  "Scans board issues for status changes in the lookback period and sends a summary to Slack.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := cfg.Validate(); err != nil {
			return err
		}

		if cmd.Flags().Changed("lookback-days") {
			cfg.StatusReport.LookbackDays = lookbackDays
		}

		jiraClient := jira.NewClient(cfg.Jira.BaseURL, cfg.Jira.Email, cfg.Jira.APIToken, logger)
		slackClient := slack.NewClient(cfg.Slack.WebhookURL, cfg.DryRun, logger)
		reporter := statusreport.New(jiraClient, slackClient, cfg, logger)

		return reporter.Run(cmd.Context())
	},
}

func init() {
	statusReportCmd.Flags().IntVar(&lookbackDays, "lookback-days", 0, "number of days to look back for status changes (overrides config)")
}
