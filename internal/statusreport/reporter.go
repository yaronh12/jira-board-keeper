package statusreport

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
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
	lookbackDays := r.config.StatusReport.LookbackDays
	if lookbackDays == 0 {
		lookbackDays = 7
	}
	cutoff := time.Now().AddDate(0, 0, -lookbackDays)

	jql := r.buildJQL(lookbackDays)
	r.logger.Info("searching for status changes", "jql", jql, "lookback_days", lookbackDays)

	issues, err := r.jira.SearchIssues(ctx, jql, jira.SearchOptions{
		ExpandChangelog: true,
	})
	if err != nil {
		// Fall back to searching without changelog expansion and fetching individually
		r.logger.Warn("search with changelog failed, falling back to individual fetches", "error", err)
		return r.runWithIndividualFetches(ctx, lookbackDays, cutoff)
	}

	// Issues from search with expand=changelog won't have changelog in our domain type,
	// so we need to fetch status changes individually.
	// The JQL already filtered to issues that had status changes, so we just need the details.
	var allChanges []jira.StatusChange
	for _, issue := range issues {
		changes, err := r.jira.GetStatusChanges(ctx, issue.Key)
		if err != nil {
			r.logger.Error("failed to get status changes", "issue", issue.Key, "error", err)
			continue
		}
		for _, c := range changes {
			if c.ChangedAt.After(cutoff) {
				allChanges = append(allChanges, c)
			}
		}
	}

	sort.Slice(allChanges, func(i, j int) bool {
		return allChanges[i].ChangedAt.After(allChanges[j].ChangedAt)
	})

	r.logger.Info("found status changes", "count", len(allChanges))

	msg := slack.FormatStatusReport(r.config.Team.Name, allChanges, strings.TrimRight(r.config.Jira.BaseURL, "/"))
	return r.slack.Send(ctx, msg)
}

func (r *Reporter) runWithIndividualFetches(ctx context.Context, lookbackDays int, cutoff time.Time) error {
	jql := r.buildBaseJQL()
	issues, err := r.jira.SearchIssues(ctx, jql, jira.SearchOptions{
		Fields: []string{"key", "summary", "status", "issuetype", "assignee"},
	})
	if err != nil {
		return fmt.Errorf("searching issues: %w", err)
	}

	var allChanges []jira.StatusChange
	for _, issue := range issues {
		changes, err := r.jira.GetStatusChanges(ctx, issue.Key)
		if err != nil {
			r.logger.Error("failed to get status changes", "issue", issue.Key, "error", err)
			continue
		}
		for _, c := range changes {
			if c.ChangedAt.After(cutoff) {
				allChanges = append(allChanges, c)
			}
		}
	}

	sort.Slice(allChanges, func(i, j int) bool {
		return allChanges[i].ChangedAt.After(allChanges[j].ChangedAt)
	})

	r.logger.Info("found status changes", "count", len(allChanges))
	msg := slack.FormatStatusReport(r.config.Team.Name, allChanges, strings.TrimRight(r.config.Jira.BaseURL, "/"))
	return r.slack.Send(ctx, msg)
}

func (r *Reporter) buildJQL(lookbackDays int) string {
	base := r.buildBaseJQL()
	return fmt.Sprintf("%s AND status CHANGED AFTER \"-%dd\"", base, lookbackDays)
}

func (r *Reporter) buildBaseJQL() string {
	if r.config.Board.JQLFilter != "" {
		return r.config.Board.JQLFilter
	}
	return fmt.Sprintf("labels = %q", r.config.Board.Label)
}
