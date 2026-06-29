package stalereport

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/yaronhod/jira-board-keeper/internal/config"
	"github.com/yaronhod/jira-board-keeper/internal/jira"
	"github.com/yaronhod/jira-board-keeper/internal/slack"
)

type Reporter struct {
	jira   jira.Client
	slack  *slack.Client
	config *config.Config
	logger *slog.Logger
}

func New(jiraClient jira.Client, slackClient *slack.Client, cfg *config.Config, logger *slog.Logger) *Reporter {
	return &Reporter{
		jira:   jiraClient,
		slack:  slackClient,
		config: cfg,
		logger: logger,
	}
}

func (r *Reporter) Run(ctx context.Context) error {
	jql := r.buildJQL()
	r.logger.Info("searching for open issues", "jql", jql)

	issues, err := r.jira.SearchIssues(ctx, jql, jira.SearchOptions{
		Fields: []string{"key", "summary", "status", "issuetype", "assignee", "updated"},
	})
	if err != nil {
		return fmt.Errorf("searching issues: %w", err)
	}

	r.logger.Info("found open issues", "count", len(issues))

	now := time.Now()
	staleByType := make(map[string][]slack.StaleIssue)

	for _, issue := range issues {
		if !r.config.IsStaleReportIssueType(issue.Type) {
			continue
		}

		threshold := r.config.GetStaleThreshold(issue.Type)
		cutoff := now.AddDate(0, 0, -threshold)

		if issue.Updated.After(cutoff) {
			continue
		}

		daysSince := int(now.Sub(issue.Updated).Hours() / 24)
		staleByType[issue.Type] = append(staleByType[issue.Type], slack.StaleIssue{
			Issue:           issue,
			DaysSinceChange: daysSince,
			LastChangeDate:  issue.Updated.Format("2006-01-02"),
		})
	}

	total := 0
	for _, issues := range staleByType {
		total += len(issues)
	}
	r.logger.Info("found stale issues", "count", total)

	msg := slack.FormatStaleReport(r.config.Team.Name, staleByType, strings.TrimRight(r.config.Jira.BaseURL, "/"))
	return r.slack.Send(ctx, msg)
}

func (r *Reporter) buildJQL() string {
	if r.config.Board.JQLFilter != "" {
		return fmt.Sprintf("%s AND statusCategory != Done", r.config.Board.JQLFilter)
	}
	return fmt.Sprintf("labels = %q AND statusCategory != Done", r.config.Board.Label)
}
