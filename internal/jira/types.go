package jira

import "time"

type Issue struct {
	Key      string
	Summary  string
	Type     string
	Status   string
	Assignee string
	Reporter string
	Labels   []string
	Updated  time.Time
	Created  time.Time
}

type StatusChange struct {
	IssueKey     string
	IssueSummary string
	IssueType    string
	Assignee     string
	FromStatus   string
	ToStatus     string
	ChangedAt    time.Time
	Author       string
}

type SearchOptions struct {
	MaxResults      int
	Fields          []string
	ExpandChangelog bool
}
