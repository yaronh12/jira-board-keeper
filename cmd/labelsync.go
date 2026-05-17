package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yaronhod/jira-board-reporter/internal/jira"
	"github.com/yaronhod/jira-board-reporter/internal/labelsync"
)

var labelSyncCmd = &cobra.Command{
	Use:   "label-sync",
	Short: "Sync team labels on Jira issues",
	Long:  "Scans Jira for issues where team members are assignee or reporter, and adds the configured label.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := cfg.Validate(); err != nil {
			return err
		}

		jiraClient := jira.NewClient(cfg.Jira.BaseURL, cfg.Jira.Email, cfg.Jira.APIToken, logger)
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

		return nil
	},
}
