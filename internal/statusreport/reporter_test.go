package statusreport

import (
	"testing"
	"time"

	"github.com/yaronhod/jira-board-keeper/internal/jira"
)

func TestCollapseByIssue_SingleTransition(t *testing.T) {
	changes := []jira.StatusChange{
		{IssueKey: "CNF-1", FromStatus: "To Do", ToStatus: "In Progress", ChangedAt: time.Now()},
	}

	got := collapseByIssue(changes)
	if len(got) != 1 {
		t.Fatalf("expected 1 change, got %d", len(got))
	}
	if got[0].FromStatus != "To Do" || got[0].ToStatus != "In Progress" {
		t.Fatalf("expected To Do → In Progress, got %s → %s", got[0].FromStatus, got[0].ToStatus)
	}
}

func TestCollapseByIssue_MultipleTransitions(t *testing.T) {
	now := time.Now()
	changes := []jira.StatusChange{
		{IssueKey: "CNF-1", FromStatus: "To Do", ToStatus: "In Progress", ChangedAt: now.Add(-2 * time.Hour)},
		{IssueKey: "CNF-1", FromStatus: "In Progress", ToStatus: "Review", ChangedAt: now.Add(-1 * time.Hour)},
	}

	got := collapseByIssue(changes)
	if len(got) != 1 {
		t.Fatalf("expected 1 collapsed change, got %d", len(got))
	}
	if got[0].FromStatus != "To Do" || got[0].ToStatus != "Review" {
		t.Fatalf("expected To Do → Review, got %s → %s", got[0].FromStatus, got[0].ToStatus)
	}
	if !got[0].ChangedAt.Equal(now.Add(-1 * time.Hour)) {
		t.Fatal("expected ChangedAt to be the latest transition time")
	}
}

func TestCollapseByIssue_Bounceback(t *testing.T) {
	now := time.Now()
	changes := []jira.StatusChange{
		{IssueKey: "CNF-1", FromStatus: "Review", ToStatus: "In Progress", ChangedAt: now.Add(-2 * time.Hour)},
		{IssueKey: "CNF-1", FromStatus: "In Progress", ToStatus: "Review", ChangedAt: now.Add(-1 * time.Hour)},
	}

	got := collapseByIssue(changes)
	if len(got) != 0 {
		t.Fatalf("expected 0 changes (no-op bounceback), got %d", len(got))
	}
}

func TestCollapseByIssue_MultipleIssues(t *testing.T) {
	now := time.Now()
	changes := []jira.StatusChange{
		{IssueKey: "CNF-1", FromStatus: "To Do", ToStatus: "In Progress", ChangedAt: now.Add(-3 * time.Hour)},
		{IssueKey: "CNF-1", FromStatus: "In Progress", ToStatus: "Review", ChangedAt: now.Add(-2 * time.Hour)},
		{IssueKey: "CNF-2", FromStatus: "In Progress", ToStatus: "Done", ChangedAt: now.Add(-1 * time.Hour)},
	}

	got := collapseByIssue(changes)
	if len(got) != 2 {
		t.Fatalf("expected 2 changes (one per issue), got %d", len(got))
	}
	if got[0].IssueKey != "CNF-1" || got[0].FromStatus != "To Do" || got[0].ToStatus != "Review" {
		t.Fatalf("CNF-1: expected To Do → Review, got %s → %s", got[0].FromStatus, got[0].ToStatus)
	}
	if got[1].IssueKey != "CNF-2" || got[1].FromStatus != "In Progress" || got[1].ToStatus != "Done" {
		t.Fatalf("CNF-2: expected In Progress → Done, got %s → %s", got[1].FromStatus, got[1].ToStatus)
	}
}

func TestCollapseByIssue_PreservesLatestMetadata(t *testing.T) {
	now := time.Now()
	changes := []jira.StatusChange{
		{
			IssueKey: "CNF-1", IssueSummary: "Old summary", Assignee: "Alice",
			FromStatus: "To Do", ToStatus: "In Progress", ChangedAt: now.Add(-2 * time.Hour),
		},
		{
			IssueKey: "CNF-1", IssueSummary: "Updated summary", Assignee: "Bob",
			FromStatus: "In Progress", ToStatus: "Review", ChangedAt: now.Add(-1 * time.Hour),
		},
	}

	got := collapseByIssue(changes)
	if len(got) != 1 {
		t.Fatalf("expected 1 change, got %d", len(got))
	}
	if got[0].IssueSummary != "Updated summary" {
		t.Fatalf("expected latest summary, got %q", got[0].IssueSummary)
	}
	if got[0].Assignee != "Bob" {
		t.Fatalf("expected latest assignee, got %q", got[0].Assignee)
	}
}

func TestCollapseByIssue_Empty(t *testing.T) {
	got := collapseByIssue(nil)
	if len(got) != 0 {
		t.Fatalf("expected 0 changes for nil input, got %d", len(got))
	}
}

func TestCollapseByIssue_UnorderedInput(t *testing.T) {
	now := time.Now()
	changes := []jira.StatusChange{
		{IssueKey: "CNF-1", FromStatus: "In Progress", ToStatus: "Review", ChangedAt: now.Add(-1 * time.Hour)},
		{IssueKey: "CNF-1", FromStatus: "To Do", ToStatus: "In Progress", ChangedAt: now.Add(-3 * time.Hour)},
		{IssueKey: "CNF-1", FromStatus: "Review", ToStatus: "Done", ChangedAt: now.Add(-30 * time.Minute)},
	}

	got := collapseByIssue(changes)
	if len(got) != 1 {
		t.Fatalf("expected 1 change, got %d", len(got))
	}
	if got[0].FromStatus != "To Do" || got[0].ToStatus != "Done" {
		t.Fatalf("expected To Do → Done, got %s → %s", got[0].FromStatus, got[0].ToStatus)
	}
}
