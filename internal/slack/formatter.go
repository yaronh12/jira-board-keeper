package slack

import (
	"fmt"
	"strings"

	"github.com/yaronhod/jira-board-keeper/internal/jira"
)

type Message struct {
	Text   string  `json:"text"`
	Blocks []Block `json:"blocks,omitempty"`
}

type Block struct {
	Type string      `json:"type"`
	Text *TextObject `json:"text,omitempty"`
}

type TextObject struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func header(text string) Block {
	return Block{
		Type: "header",
		Text: &TextObject{Type: "plain_text", Text: text},
	}
}

func divider() Block {
	return Block{Type: "divider"}
}

func section(mrkdwn string) Block {
	return Block{
		Type: "section",
		Text: &TextObject{Type: "mrkdwn", Text: mrkdwn},
	}
}

func FormatStatusReport(teamName string, changes []jira.StatusChange, jiraBaseURL string) *Message {
	fallback := fmt.Sprintf("Weekly Status Report - %s: %d status changes", teamName, len(changes))

	blocks := []Block{
		header(fmt.Sprintf("Weekly Status Report - %s", teamName)),
		divider(),
	}

	if len(changes) == 0 {
		blocks = append(blocks, section("No status changes this week."))
		return &Message{Text: fallback, Blocks: blocks}
	}

	var lines []string
	for _, c := range changes {
		issueLink := fmt.Sprintf("<%s/browse/%s|%s>", jiraBaseURL, c.IssueKey, c.IssueKey)
		assignee := c.Assignee
		if assignee == "" {
			assignee = "Unassigned"
		}
		line := fmt.Sprintf("*%s* %s\n     _%s_ → *%s* | %s",
			issueLink, c.IssueSummary, c.FromStatus, c.ToStatus, assignee)
		lines = append(lines, line)
	}

	// Slack blocks have a 3000 char limit per text field — split if needed
	chunk := ""
	for _, line := range lines {
		if len(chunk)+len(line)+1 > 2900 {
			blocks = append(blocks, section(chunk))
			chunk = ""
		}
		if chunk != "" {
			chunk += "\n\n"
		}
		chunk += line
	}
	if chunk != "" {
		blocks = append(blocks, section(chunk))
	}

	return &Message{Text: fallback, Blocks: blocks}
}

type LabeledIssue struct {
	Key     string
	Summary string
}

func FormatLabelSyncReport(teamName string, totalFound, alreadyLabeled int, labeledIssues []LabeledIssue, jiraBaseURL string) *Message {
	fallback := fmt.Sprintf("Label Sync - %s: %d newly labeled", teamName, len(labeledIssues))

	blocks := []Block{
		header(fmt.Sprintf("Label Sync - %s", teamName)),
		divider(),
		section(fmt.Sprintf("Scanned %d issues | %d already labeled | *%d newly labeled*",
			totalFound, alreadyLabeled, len(labeledIssues))),
	}

	var lines []string
	for _, issue := range labeledIssues {
		issueLink := fmt.Sprintf("<%s/browse/%s|%s>", jiraBaseURL, issue.Key, issue.Key)
		lines = append(lines, fmt.Sprintf("• %s %s", issueLink, issue.Summary))
	}

	chunk := strings.Join(lines, "\n")
	if len(chunk) > 2900 {
		current := ""
		for _, line := range lines {
			if len(current)+len(line)+1 > 2900 {
				blocks = append(blocks, section(current))
				current = ""
			}
			if current != "" {
				current += "\n"
			}
			current += line
		}
		if current != "" {
			blocks = append(blocks, section(current))
		}
	} else {
		blocks = append(blocks, section(chunk))
	}

	return &Message{Text: fallback, Blocks: blocks}
}

type StaleIssue struct {
	Issue          jira.Issue
	DaysSinceChange int
	LastChangeDate  string
}

func FormatStaleReport(teamName string, staleByType map[string][]StaleIssue, jiraBaseURL string) *Message {
	total := 0
	for _, issues := range staleByType {
		total += len(issues)
	}

	fallback := fmt.Sprintf("Stale Issue Report - %s: %d stale issues", teamName, total)

	blocks := []Block{
		header(fmt.Sprintf("Stale Issue Report - %s", teamName)),
		divider(),
	}

	if total == 0 {
		blocks = append(blocks, section("No stale issues found."))
		return &Message{Text: fallback, Blocks: blocks}
	}

	typeOrder := []string{"Epic", "Story", "Task", "Bug", "Sub-task"}
	for _, t := range typeOrder {
		issues, ok := staleByType[t]
		if !ok || len(issues) == 0 {
			continue
		}
		blocks = append(blocks, section(fmt.Sprintf("*Stale %ss (%d)*", t, len(issues))))

		var lines []string
		for _, si := range issues {
			issueLink := fmt.Sprintf("<%s/browse/%s|%s>", jiraBaseURL, si.Issue.Key, si.Issue.Key)
			assignee := si.Issue.Assignee
			if assignee == "" {
				assignee = "Unassigned"
			}
			lines = append(lines, fmt.Sprintf("• %s %s | %s | Last change: %s (%d days ago)",
				issueLink, si.Issue.Summary, assignee, si.LastChangeDate, si.DaysSinceChange))
		}

		chunk := strings.Join(lines, "\n")
		if len(chunk) > 2900 {
			// Split into multiple blocks
			current := ""
			for _, line := range lines {
				if len(current)+len(line)+1 > 2900 {
					blocks = append(blocks, section(current))
					current = ""
				}
				if current != "" {
					current += "\n"
				}
				current += line
			}
			if current != "" {
				blocks = append(blocks, section(current))
			}
		} else {
			blocks = append(blocks, section(chunk))
		}
	}

	// Handle any types not in the predefined order
	for t, issues := range staleByType {
		found := false
		for _, ordered := range typeOrder {
			if t == ordered {
				found = true
				break
			}
		}
		if found || len(issues) == 0 {
			continue
		}
		blocks = append(blocks, section(fmt.Sprintf("*Stale %ss (%d)*", t, len(issues))))
		var lines []string
		for _, si := range issues {
			issueLink := fmt.Sprintf("<%s/browse/%s|%s>", jiraBaseURL, si.Issue.Key, si.Issue.Key)
			lines = append(lines, fmt.Sprintf("• %s %s | Last change: %s (%d days ago)",
				issueLink, si.Issue.Summary, si.LastChangeDate, si.DaysSinceChange))
		}
		blocks = append(blocks, section(strings.Join(lines, "\n")))
	}

	return &Message{Text: fallback, Blocks: blocks}
}
