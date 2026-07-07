package labelsync

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/yaronhod/jira-board-keeper/internal/config"
	"github.com/yaronhod/jira-board-keeper/internal/jira"
)

var reviewCandidateTypes = map[string]bool{
	"epic":    true,
	"feature": true,
}

type LabeledIssue struct {
	Key     string
	Summary string
}

type SyncResult struct {
	TotalFound       int
	AlreadyLabeled   int
	NewlyLabeled     int
	Errors           int
	LabeledIssues    []LabeledIssue
	ReviewCandidates []string
}

type Syncer struct {
	jira   jira.Client
	config *config.Config
	logger *slog.Logger
}

func New(jiraClient jira.Client, cfg *config.Config, logger *slog.Logger) *Syncer {
	return &Syncer{
		jira:   jiraClient,
		config: cfg,
		logger: logger,
	}
}

func (s *Syncer) Run(ctx context.Context) (*SyncResult, error) {
	jql := s.buildJQL()
	s.logger.Info("searching for team issues", "jql", jql)

	issues, err := s.jira.SearchIssues(ctx, jql, jira.SearchOptions{
		Fields: []string{"key", "labels", "summary", "assignee", "reporter", "issuetype"},
	})
	if err != nil {
		return nil, fmt.Errorf("searching issues: %w", err)
	}

	result := &SyncResult{TotalFound: len(issues)}
	s.logger.Info("found issues", "count", len(issues))

	for _, issue := range issues {
		if hasLabel(issue.Labels, s.config.Board.Label) {
			result.AlreadyLabeled++
			continue
		}

		s.logger.Info("adding label", "issue", issue.Key, "label", s.config.Board.Label)

		if s.config.DryRun {
			s.logger.Info("dry-run: would add label", "issue", issue.Key)
			result.NewlyLabeled++
			result.LabeledIssues = append(result.LabeledIssues, LabeledIssue{Key: issue.Key, Summary: issue.Summary})
			continue
		}

		if err := s.jira.AddLabel(ctx, issue.Key, s.config.Board.Label); err != nil {
			s.logger.Error("failed to add label", "issue", issue.Key, "error", err)
			result.Errors++
			continue
		}
		result.NewlyLabeled++
		result.LabeledIssues = append(result.LabeledIssues, LabeledIssue{Key: issue.Key, Summary: issue.Summary})
	}

	for _, issue := range issues {
		if reviewCandidateTypes[strings.ToLower(issue.Type)] {
			result.ReviewCandidates = append(result.ReviewCandidates, issue.Key)
		}
	}

	return result, nil
}

func (s *Syncer) buildJQL() string {
	quoted := make([]string, len(s.config.Team.Members))
	for i, m := range s.config.Team.Members {
		quoted[i] = fmt.Sprintf("%q", m)
	}
	members := strings.Join(quoted, ", ")

	jql := fmt.Sprintf("assignee in (%s)", members)

	lookback := s.config.LabelSync.LookbackDays
	if lookback == 0 {
		lookback = 7
	}
	jql += fmt.Sprintf(" AND updated >= -%dd", lookback)

	if len(s.config.Board.ProjectKeys) > 0 {
		quotedProjects := make([]string, len(s.config.Board.ProjectKeys))
		for i, p := range s.config.Board.ProjectKeys {
			quotedProjects[i] = fmt.Sprintf("%q", p)
		}
		jql += fmt.Sprintf(" AND project in (%s)", strings.Join(quotedProjects, ", "))
	}

	return jql
}

func hasLabel(labels []string, target string) bool {
	for _, l := range labels {
		if strings.EqualFold(l, target) {
			return true
		}
	}
	return false
}
