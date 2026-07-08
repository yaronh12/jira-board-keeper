package labelsync

import (
	"context"
	"fmt"
	"log/slog"
	"io"
	"testing"

	"github.com/yaronhod/jira-board-keeper/internal/config"
	"github.com/yaronhod/jira-board-keeper/internal/jira"
)

type mockJiraClient struct {
	issues      []jira.Issue
	searchErr   error
	addLabelErr error
	labeled     []string
}

func (m *mockJiraClient) SearchIssues(_ context.Context, _ string, _ jira.SearchOptions) ([]jira.Issue, error) {
	return m.issues, m.searchErr
}

func (m *mockJiraClient) AddLabel(_ context.Context, issueKey string, _ string) error {
	if m.addLabelErr != nil {
		return m.addLabelErr
	}
	m.labeled = append(m.labeled, issueKey)
	return nil
}

func (m *mockJiraClient) GetStatusChanges(_ context.Context, _ string) ([]jira.StatusChange, error) {
	return nil, nil
}

func (m *mockJiraClient) GetCurrentUser(_ context.Context) (string, error) {
	return "test (test@example.com)", nil
}

func TestRun_AllAlreadyLabeled(t *testing.T) {
	mock := &mockJiraClient{
		issues: []jira.Issue{
			{Key: "TEST-1", Labels: []string{"team-label"}},
			{Key: "TEST-2", Labels: []string{"other", "team-label"}},
		},
	}
	cfg := &config.Config{
		Team:  config.TeamConfig{Members: map[string]string{"user1": ""}},
		Board: config.BoardConfig{Label: "team-label"},
	}

	syncer := New(mock, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	result, err := syncer.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalFound != 2 || result.AlreadyLabeled != 2 || result.NewlyLabeled != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRun_MixedLabeled(t *testing.T) {
	mock := &mockJiraClient{
		issues: []jira.Issue{
			{Key: "TEST-1", Labels: []string{"team-label"}},
			{Key: "TEST-2", Labels: []string{"other"}},
			{Key: "TEST-3", Labels: []string{}},
		},
	}
	cfg := &config.Config{
		Team:  config.TeamConfig{Members: map[string]string{"user1": ""}},
		Board: config.BoardConfig{Label: "team-label"},
	}

	syncer := New(mock, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	result, err := syncer.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AlreadyLabeled != 1 || result.NewlyLabeled != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(mock.labeled) != 2 {
		t.Fatalf("expected 2 AddLabel calls, got %d", len(mock.labeled))
	}
}

func TestRun_DryRun(t *testing.T) {
	mock := &mockJiraClient{
		issues: []jira.Issue{
			{Key: "TEST-1", Labels: []string{}},
		},
	}
	cfg := &config.Config{
		Team:   config.TeamConfig{Members: map[string]string{"user1": ""}},
		Board:  config.BoardConfig{Label: "team-label"},
		DryRun: true,
	}

	syncer := New(mock, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	result, err := syncer.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.NewlyLabeled != 1 {
		t.Fatalf("expected 1 newly labeled in dry-run, got %d", result.NewlyLabeled)
	}
	if len(mock.labeled) != 0 {
		t.Fatal("should not call AddLabel in dry-run mode")
	}
}

func TestRun_ReviewCandidates(t *testing.T) {
	mock := &mockJiraClient{
		issues: []jira.Issue{
			{Key: "CNF-100", Type: "Epic", Labels: []string{"team-label"}},
			{Key: "CNF-101", Type: "Story", Labels: []string{"team-label"}},
			{Key: "CNF-102", Type: "Feature", Labels: []string{}},
			{Key: "CNF-103", Type: "Bug", Labels: []string{}},
			{Key: "CNF-104", Type: "epic", Labels: []string{"team-label"}},
		},
	}
	cfg := &config.Config{
		Team:  config.TeamConfig{Members: map[string]string{"user1": ""}},
		Board: config.BoardConfig{Label: "team-label"},
	}

	syncer := New(mock, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	result, err := syncer.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.ReviewCandidates) != 3 {
		t.Fatalf("expected 3 review candidates (2 epics + 1 feature), got %d: %v",
			len(result.ReviewCandidates), result.ReviewCandidates)
	}
	expected := map[string]bool{"CNF-100": true, "CNF-102": true, "CNF-104": true}
	for _, key := range result.ReviewCandidates {
		if !expected[key] {
			t.Errorf("unexpected review candidate: %s", key)
		}
	}
}

func TestRun_AddLabelError(t *testing.T) {
	mock := &mockJiraClient{
		issues: []jira.Issue{
			{Key: "TEST-1", Labels: []string{}},
		},
		addLabelErr: fmt.Errorf("permission denied"),
	}
	cfg := &config.Config{
		Team:  config.TeamConfig{Members: map[string]string{"user1": ""}},
		Board: config.BoardConfig{Label: "team-label"},
	}

	syncer := New(mock, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	result, err := syncer.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Errors != 1 {
		t.Fatalf("expected 1 error, got %d", result.Errors)
	}
}
