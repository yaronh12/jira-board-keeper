package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type Client struct {
	webhookURL string
	httpClient *http.Client
	dryRun     bool
	logger     *slog.Logger
}

func NewClient(webhookURL string, dryRun bool, logger *slog.Logger) *Client {
	return &Client{
		webhookURL: webhookURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		dryRun:     dryRun,
		logger:     logger,
	}
}

func (c *Client) Send(ctx context.Context, msg *Message) error {
	payload, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling slack message: %w", err)
	}

	if c.dryRun {
		c.logger.Info("dry-run: would send slack message", "payload", string(payload))
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.webhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("creating slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending slack message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack webhook returned status %d", resp.StatusCode)
	}

	c.logger.Info("slack message sent successfully")
	return nil
}
