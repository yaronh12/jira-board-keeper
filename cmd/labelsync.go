package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yaronhod/jira-board-keeper/internal/jira"
	"github.com/yaronhod/jira-board-keeper/internal/labelsync"
	"github.com/yaronhod/jira-board-keeper/internal/slack"
)

var labelSyncCmd = &cobra.Command{
	Use:   "label-sync",
	Short: "Sync team labels on Jira issues",
	Long:  "Scans Jira for issues where team members are assignee or reporter, and adds the configured label.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !cfg.LabelSync.Enabled && !forceRun {
			logger.Info("label-sync is disabled in config, skipping (use --force to override)")
			return nil
		}

		if err := cfg.Validate(); err != nil {
			return err
		}

		jiraClient := jira.NewClient(cfg.Jira.BaseURL, cfg.Jira.Email, cfg.Jira.APIToken, logger)

		user, err := jiraClient.GetCurrentUser(cmd.Context())
		if err != nil {
			logger.Warn("could not verify authenticated user", "error", err)
		} else {
			logger.Info("authenticated as", "user", user)
		}

		syncer := labelsync.New(jiraClient, cfg, logger)

		result, err := syncer.Run(cmd.Context())
		if err != nil {
			return fmt.Errorf("label sync failed: %w", err)
		}

		logger.Info("label sync complete",
			"found", result.TotalFound,
			"already_labeled", result.AlreadyLabeled,
			"newly_labeled", result.NewlyLabeled,
			"errors", result.Errors)

		if cfg.Slack.Enabled && result.NewlyLabeled > 0 {
			slackClient := slack.NewClient(cfg.Slack.WebhookURL, cfg.DryRun, logger)
			labeledIssues := make([]slack.LabeledIssue, len(result.LabeledIssues))
			for i, li := range result.LabeledIssues {
				labeledIssues[i] = slack.LabeledIssue{Key: li.Key, Summary: li.Summary}
			}
			jiraBaseURL := strings.TrimRight(cfg.Jira.BaseURL, "/")
			msg := slack.FormatLabelSyncReport(cfg.Team.Name,
				result.TotalFound, result.AlreadyLabeled, labeledIssues, jiraBaseURL)
			if err := slackClient.Send(cmd.Context(), msg); err != nil {
				logger.Error("failed to send slack notification", "error", err)
			}
		}

		return nil
	},
}
