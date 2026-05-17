package stalereport

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/yaronhod/jira-board-reporter/internal/config"
	"github.com/yaronhod/jira-board-reporter/internal/jira"
	"github.com/yaronhod/jira-board-reporter/internal/slack"
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
		Fields: []string{"key", "summary", "status", "issuetype", "assignee", "created"},
	})
	if err != nil {
		return fmt.Errorf("searching issues: %w", err)
	}

	r.logger.Info("found open issues", "count", len(issues))

	now := time.Now()
	staleByType := make(map[string][]slack.StaleIssue)

	for _, issue := range issues {
		threshold := r.config.GetStaleThreshold(issue.Type)
		cutoff := now.AddDate(0, 0, -threshold)

		lastChange, err := r.getLastStatusChangeDate(ctx, issue)
		if err != nil {
			r.logger.Error("failed to get changelog", "issue", issue.Key, "error", err)
			continue
		}

		if lastChange.After(cutoff) {
			continue
		}

		daysSince := int(now.Sub(lastChange).Hours() / 24)
		staleByType[issue.Type] = append(staleByType[issue.Type], slack.StaleIssue{
			Issue:           issue,
			DaysSinceChange: daysSince,
			LastChangeDate:  lastChange.Format("2006-01-02"),
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

func (r *Reporter) getLastStatusChangeDate(ctx context.Context, issue jira.Issue) (time.Time, error) {
	changes, err := r.jira.GetStatusChanges(ctx, issue.Key)
	if err != nil {
		return time.Time{}, err
	}

	if len(changes) == 0 {
		return issue.Created, nil
	}

	latest := changes[0].ChangedAt
	for _, c := range changes[1:] {
		if c.ChangedAt.After(latest) {
			latest = c.ChangedAt
		}
	}
	return latest, nil
}

func (r *Reporter) buildJQL() string {
	if r.config.Board.JQLFilter != "" {
		return fmt.Sprintf("%s AND statusCategory != Done", r.config.Board.JQLFilter)
	}
	return fmt.Sprintf("labels = %q AND statusCategory != Done", r.config.Board.Label)
}
