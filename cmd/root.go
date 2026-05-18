package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/yaronhod/jira-board-keeper/internal/config"
)

var (
	cfgFile string
	dryRun  bool
	logLevel string
	cfg     *config.Config
	logger  *slog.Logger
)

var rootCmd = &cobra.Command{
	Use:   "jira-board-keeper",
	Short: "Automated Jira board management and Slack reporting",
	Long:  "A tool that syncs team labels on Jira issues, sends weekly status reports, and detects stale issues.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		if cmd.Flags().Changed("dry-run") {
			cfg.DryRun = dryRun
		}
		if cmd.Flags().Changed("log-level") {
			cfg.LogLevel = logLevel
		}

		level := parseLogLevel(cfg.LogLevel)
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

		return nil
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "config.yaml", "config file path")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "skip writes to Jira and Slack")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "", "log level (debug, info, warn, error)")

	rootCmd.AddCommand(labelSyncCmd)
	rootCmd.AddCommand(statusReportCmd)
	rootCmd.AddCommand(staleReportCmd)
}

func parseLogLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
