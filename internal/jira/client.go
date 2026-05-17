package jira

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client interface {
	SearchIssues(ctx context.Context, jql string, opts SearchOptions) ([]Issue, error)
	AddLabel(ctx context.Context, issueKey string, label string) error
	GetStatusChanges(ctx context.Context, issueKey string) ([]StatusChange, error)
}

type httpClient struct {
	baseURL    string
	email      string
	apiToken   string
	httpClient *http.Client
	logger     *slog.Logger
}

func NewClient(baseURL, email, apiToken string, logger *slog.Logger) Client {
	return &httpClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		email:      email,
		apiToken:   apiToken,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		logger:     logger,
	}
}

func (c *httpClient) doRequest(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	reqURL := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, reqURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.SetBasicAuth(c.email, c.apiToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("jira API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

type searchResponse struct {
	StartAt    int              `json:"startAt"`
	MaxResults int              `json:"maxResults"`
	Total      int              `json:"total"`
	Issues     []jiraIssueJSON  `json:"issues"`
}

type jiraIssueJSON struct {
	Key       string          `json:"key"`
	Fields    jiraFieldsJSON  `json:"fields"`
	Changelog *changelogJSON  `json:"changelog,omitempty"`
}

type jiraFieldsJSON struct {
	Summary   string          `json:"summary"`
	Status    *statusJSON     `json:"status"`
	IssueType *issueTypeJSON  `json:"issuetype"`
	Assignee  *userJSON       `json:"assignee"`
	Reporter  *userJSON       `json:"reporter"`
	Labels    []string        `json:"labels"`
	Updated   string          `json:"updated"`
	Created   string          `json:"created"`
}

type statusJSON struct {
	Name string `json:"name"`
}

type issueTypeJSON struct {
	Name string `json:"name"`
}

type userJSON struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
}

type changelogJSON struct {
	Histories []historyJSON `json:"histories"`
}

type historyJSON struct {
	Created string      `json:"created"`
	Author  *userJSON   `json:"author"`
	Items   []itemJSON  `json:"items"`
}

type itemJSON struct {
	Field      string `json:"field"`
	FromString string `json:"fromString"`
	ToString   string `json:"toString"`
}

func (c *httpClient) SearchIssues(ctx context.Context, jql string, opts SearchOptions) ([]Issue, error) {
	maxResults := opts.MaxResults
	if maxResults == 0 {
		maxResults = 50
	}

	var allIssues []Issue
	startAt := 0

	for {
		params := url.Values{
			"jql":        {jql},
			"startAt":    {fmt.Sprintf("%d", startAt)},
			"maxResults": {fmt.Sprintf("%d", maxResults)},
		}
		if len(opts.Fields) > 0 {
			params.Set("fields", strings.Join(opts.Fields, ","))
		}
		if opts.ExpandChangelog {
			params.Set("expand", "changelog")
		}

		path := "/rest/api/3/search/jql?" + params.Encode()
		respBody, err := c.doRequest(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, fmt.Errorf("searching issues: %w", err)
		}

		var result searchResponse
		if err := json.Unmarshal(respBody, &result); err != nil {
			return nil, fmt.Errorf("unmarshaling search response: %w", err)
		}

		for _, ji := range result.Issues {
			allIssues = append(allIssues, convertIssue(ji))
		}

		c.logger.Debug("fetched issues page",
			"startAt", startAt,
			"returned", len(result.Issues),
			"total", result.Total)

		startAt += len(result.Issues)
		if startAt >= result.Total || len(result.Issues) == 0 {
			break
		}
	}

	return allIssues, nil
}

func (c *httpClient) AddLabel(ctx context.Context, issueKey string, label string) error {
	body := map[string]interface{}{
		"update": map[string]interface{}{
			"labels": []map[string]string{
				{"add": label},
			},
		},
	}

	path := fmt.Sprintf("/rest/api/3/issue/%s", issueKey)
	_, err := c.doRequest(ctx, http.MethodPut, path, body)
	if err != nil {
		return fmt.Errorf("adding label to %s: %w", issueKey, err)
	}
	return nil
}

func (c *httpClient) GetStatusChanges(ctx context.Context, issueKey string) ([]StatusChange, error) {
	path := fmt.Sprintf("/rest/api/3/issue/%s?expand=changelog", issueKey)
	respBody, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("getting issue %s: %w", issueKey, err)
	}

	var ji jiraIssueJSON
	if err := json.Unmarshal(respBody, &ji); err != nil {
		return nil, fmt.Errorf("unmarshaling issue %s: %w", issueKey, err)
	}

	return extractStatusChanges(ji), nil
}

func extractStatusChanges(ji jiraIssueJSON) []StatusChange {
	if ji.Changelog == nil {
		return nil
	}

	var changes []StatusChange
	for _, history := range ji.Changelog.Histories {
		changedAt, _ := parseJiraTime(history.Created)
		author := ""
		if history.Author != nil {
			author = history.Author.DisplayName
		}

		for _, item := range history.Items {
			if item.Field != "status" {
				continue
			}
			assignee := ""
			if ji.Fields.Assignee != nil {
				assignee = ji.Fields.Assignee.DisplayName
			}
			issueType := ""
			if ji.Fields.IssueType != nil {
				issueType = ji.Fields.IssueType.Name
			}

			changes = append(changes, StatusChange{
				IssueKey:     ji.Key,
				IssueSummary: ji.Fields.Summary,
				IssueType:    issueType,
				Assignee:     assignee,
				FromStatus:   item.FromString,
				ToStatus:     item.ToString,
				ChangedAt:    changedAt,
				Author:       author,
			})
		}
	}
	return changes
}

func convertIssue(ji jiraIssueJSON) Issue {
	updated, _ := parseJiraTime(ji.Fields.Updated)
	created, _ := parseJiraTime(ji.Fields.Created)

	issue := Issue{
		Key:     ji.Key,
		Summary: ji.Fields.Summary,
		Labels:  ji.Fields.Labels,
		Updated: updated,
		Created: created,
	}
	if ji.Fields.Status != nil {
		issue.Status = ji.Fields.Status.Name
	}
	if ji.Fields.IssueType != nil {
		issue.Type = ji.Fields.IssueType.Name
	}
	if ji.Fields.Assignee != nil {
		issue.Assignee = ji.Fields.Assignee.DisplayName
	}
	if ji.Fields.Reporter != nil {
		issue.Reporter = ji.Fields.Reporter.DisplayName
	}
	return issue
}

func parseJiraTime(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05-0700",
		time.RFC3339,
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unable to parse time: %s", s)
}
