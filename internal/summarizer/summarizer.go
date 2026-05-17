package summarizer

import "context"

type Summarizer interface {
	Summarize(ctx context.Context, input SummaryInput) (string, error)
}

type SummaryInput struct {
	TeamName      string
	StatusChanges []StatusChangeEntry
	StaleIssues   []StaleIssueEntry
	ActiveIssues  []IssueEntry
	Period        string
}

type StatusChangeEntry struct {
	IssueKey   string
	Summary    string
	FromStatus string
	ToStatus   string
	Assignee   string
}

type StaleIssueEntry struct {
	IssueKey        string
	Summary         string
	Type            string
	DaysSinceChange int
}

type IssueEntry struct {
	IssueKey string
	Summary  string
	Status   string
	Assignee string
	Type     string
}

type NoOp struct{}

func (n *NoOp) Summarize(_ context.Context, _ SummaryInput) (string, error) {
	return "AI summary not yet implemented.", nil
}
