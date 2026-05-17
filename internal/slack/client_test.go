package slack

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSend_Success(t *testing.T) {
	var receivedMsg Message
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("expected application/json content type")
		}
		json.NewDecoder(r.Body).Decode(&receivedMsg)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, false, slog.New(slog.NewTextHandler(io.Discard, nil)))
	msg := &Message{Text: "test message", Blocks: []Block{{Type: "section", Text: &TextObject{Type: "mrkdwn", Text: "hello"}}}}

	err := client.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedMsg.Text != "test message" {
		t.Fatalf("expected 'test message', got %q", receivedMsg.Text)
	}
}

func TestSend_DryRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not make HTTP request in dry-run mode")
	}))
	defer server.Close()

	client := NewClient(server.URL, true, slog.New(slog.NewTextHandler(io.Discard, nil)))
	msg := &Message{Text: "test"}

	err := client.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSend_WebhookError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, false, slog.New(slog.NewTextHandler(io.Discard, nil)))
	err := client.Send(context.Background(), &Message{Text: "test"})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}
