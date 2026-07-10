# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Repository Overview

`jira-board-keeper` is an automated Jira board management and Slack reporting tool. It runs as scheduled GitHub Actions jobs to keep a team's Jira board organized and stakeholders informed.

### Features

- **Label Sync** -- Scans Jira for issues where team members are assignee or reporter, adds a configurable label so they appear on the team board
- **Status Report** -- Weekly Slack message summarizing all status changes on the board
- **Stale Report** -- Detects issues with no status change beyond configurable thresholds (e.g., 60 days for Epics, 30 for Stories)
- **Epic Style Review** -- Scores Epic/Feature descriptions using an AI agent, creates GitHub Issues for failing epics with suggested improvements, and notifies via Slack

## Build Commands

```bash
make build              # Build binary to bin/jira-board-keeper
make test               # Run tests with -v -race flags
make clean              # Remove bin/ directory
make docker-build       # Build Docker image
```

## Test Commands

```bash
make test               # Run all tests with verbose output and race detection
go test ./... -v -race  # Equivalent manual command
```

## Code Organization

```
jira-board-keeper/
├── main.go                          # Entry point, calls cmd.Execute()
├── cmd/
│   ├── root.go                      # Root cobra command, config loading, flag setup
│   ├── labelsync.go                 # label-sync subcommand
│   ├── statusreport.go             # status-report subcommand
│   └── stalereport.go              # stale-report subcommand
├── internal/
│   ├── config/
│   │   └── config.go               # Config loading (Viper), validation, defaults
│   ├── jira/
│   │   ├── client.go               # Jira REST API client (v2/v3)
│   │   ├── client_test.go          # Client tests
│   │   └── types.go                # Jira API type definitions
│   ├── slack/
│   │   ├── client.go               # Slack webhook client
│   │   ├── client_test.go          # Client tests
│   │   └── formatter.go            # Slack message formatting
│   ├── labelsync/
│   │   ├── syncer.go               # Label sync logic
│   │   └── syncer_test.go          # Syncer tests
│   ├── stalereport/
│   │   └── reporter.go             # Stale issue detection and reporting
│   ├── statusreport/
│   │   ├── reporter.go             # Status change reporting
│   │   └── reporter_test.go        # Reporter tests
│   └── summarizer/
│       └── summarizer.go           # Text summarization utilities
├── scripts/
│   ├── epic_review.py              # AI-driven epic description scoring (Cursor SDK)
│   └── requirements.txt            # Python dependencies for epic review
├── .github/workflows/
│   ├── ci.yml                      # PR/push CI (test + build)
│   ├── label-sync.yml              # Scheduled label sync + epic review
│   ├── status-report.yml           # Scheduled status report
│   ├── stale-report.yml            # Scheduled stale report
│   └── apply-review.yml            # Apply approved epic review suggestions to Jira
├── config.example.yaml             # Example configuration file
├── Dockerfile                      # Multi-stage Docker build
└── Makefile                        # Build, test, clean targets
```

## Key Dependencies

From `go.mod`:

| Dependency | Purpose |
|------------|---------|
| `github.com/spf13/cobra` | CLI command framework |
| `github.com/spf13/viper` | Configuration loading (YAML + env vars) |

## Development Guidelines

### Go Version

This project uses Go 1.26.3 (per go.mod). CI workflows currently pin Go 1.22.

### Configuration

Configuration is loaded from a YAML file (default `config.yaml`) with environment variable overrides. See `config.example.yaml` for all options.

Required environment variables:
- `JIRA_EMAIL` -- Jira account email
- `JIRA_API_TOKEN` -- Jira API token
- `SLACK_WEBHOOK_URL` -- Slack incoming webhook URL

Override hierarchy: CLI flags > Environment variables > config.yaml > defaults

### Running Locally

```bash
make build

# Dry run (no writes to Jira or Slack)
./bin/jira-board-keeper label-sync --config config.yaml --dry-run
./bin/jira-board-keeper status-report --config config.yaml --dry-run
./bin/jira-board-keeper stale-report --config config.yaml --dry-run
```

### Docker

```bash
make docker-build
docker run --rm \
  -e JIRA_EMAIL=you@example.com \
  -e JIRA_API_TOKEN=token \
  -e SLACK_WEBHOOK_URL=https://hooks.slack.com/... \
  -v $(pwd)/config.yaml:/config.yaml \
  jira-board-keeper label-sync --config /config.yaml
```

## CI/CD Workflows

### ci.yml

PR and push validation:
- **Trigger**: Push to main/master, PRs to main/master
- **Actions**: Run tests with race detection, build binary

### label-sync.yml

Scheduled label sync with optional epic review:
- **Trigger**: Monday 06:00 UTC, manual dispatch
- **Actions**: Build, run label-sync, optionally run epic style review on discovered epics

### status-report.yml / stale-report.yml

Scheduled Slack reports:
- **Trigger**: Monday 06:15 UTC, manual dispatch
- **Actions**: Build, run respective report command

### apply-review.yml

Apply approved epic descriptions to Jira:
- **Trigger**: `/approve` comment on GitHub Issues labeled `epic-review`
- **Actions**: Extract Jira key and description from issue, update Jira via REST API, close the issue
