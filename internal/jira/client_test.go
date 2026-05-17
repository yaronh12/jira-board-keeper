package jira

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchIssues_Pagination(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var resp searchResponse
		if callCount == 1 {
			resp = searchResponse{
				StartAt:    0,
				MaxResults: 2,
				Total:      3,
				Issues: []jiraIssueJSON{
					{Key: "TEST-1", Fields: jiraFieldsJSON{Summary: "Issue 1", Labels: []string{}}},
					{Key: "TEST-2", Fields: jiraFieldsJSON{Summary: "Issue 2", Labels: []string{"existing"}}},
				},
			}
		} else {
			resp = searchResponse{
				StartAt:    2,
				MaxResults: 2,
				Total:      3,
				Issues: []jiraIssueJSON{
					{Key: "TEST-3", Fields: jiraFieldsJSON{Summary: "Issue 3", Labels: []string{}}},
				},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test@example.com", "token", discardLogger())
	issues, err := client.SearchIssues(context.Background(), "project = TEST", SearchOptions{MaxResults: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(issues) != 3 {
		t.Fatalf("expected 3 issues, got %d", len(issues))
	}
	if callCount != 2 {
		t.Fatalf("expected 2 API calls for pagination, got %d", callCount)
	}
	if issues[1].Labels[0] != "existing" {
		t.Fatalf("expected label 'existing' on issue 2, got %v", issues[1].Labels)
	}
}

func TestAddLabel(t *testing.T) {
	var receivedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test@example.com", "token", discardLogger())
	err := client.AddLabel(context.Background(), "TEST-1", "my-label")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	update := receivedBody["update"].(map[string]interface{})
	labels := update["labels"].([]interface{})
	addOp := labels[0].(map[string]interface{})
	if addOp["add"] != "my-label" {
		t.Fatalf("expected label 'my-label', got %v", addOp["add"])
	}
}

func TestGetStatusChanges(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := jiraIssueJSON{
			Key: "TEST-1",
			Fields: jiraFieldsJSON{
				Summary:   "Test Issue",
				IssueType: &issueTypeJSON{Name: "Story"},
				Assignee:  &userJSON{DisplayName: "John Doe"},
			},
			Changelog: &changelogJSON{
				Histories: []historyJSON{
					{
						Created: "2025-01-15T10:00:00.000+0000",
						Author:  &userJSON{DisplayName: "Jane Smith"},
						Items: []itemJSON{
							{Field: "status", FromString: "To Do", ToString: "In Progress"},
						},
					},
					{
						Created: "2025-01-16T10:00:00.000+0000",
						Author:  &userJSON{DisplayName: "Jane Smith"},
						Items: []itemJSON{
							{Field: "priority", FromString: "Medium", ToString: "High"},
						},
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test@example.com", "token", discardLogger())
	changes, err := client.GetStatusChanges(context.Background(), "TEST-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(changes) != 1 {
		t.Fatalf("expected 1 status change (priority change filtered out), got %d", len(changes))
	}
	if changes[0].FromStatus != "To Do" || changes[0].ToStatus != "In Progress" {
		t.Fatalf("unexpected status change: %v", changes[0])
	}
}

func TestSearchIssues_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"errorMessages":["invalid JQL"]}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test@example.com", "token", discardLogger())
	_, err := client.SearchIssues(context.Background(), "bad jql", SearchOptions{})
	if err == nil {
		t.Fatal("expected error for bad JQL")
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
