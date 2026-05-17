# jira-board-reporter

Automated Jira board management and Slack reporting. Runs as scheduled GitHub Actions jobs — fork the repo, add your config and secrets, and it works for any team.

## Features

- **Label Sync** — Scans Jira for issues where team members are assignee or reporter, adds a label so they appear on your team board
- **Status Report** — Weekly Slack message summarizing all status changes on the board
- **Stale Report** — Detects issues with no status change beyond configurable thresholds (e.g., 60 days for Epics, 30 for Stories)

## Quick Start

### 1. Clone and configure

```bash
cp config.example.yaml config.yaml
# Edit config.yaml with your team members, project keys, label name, etc.
```

### 2. Set environment variables

```bash
export JIRA_EMAIL="you@example.com"
export JIRA_API_TOKEN="your-api-token"       # https://id.atlassian.com/manage-profile/security/api-tokens
export SLACK_WEBHOOK_URL="https://hooks.slack.com/services/..."
```

### 3. Build and run

```bash
make build

# Dry run (no writes to Jira or Slack)
./bin/jira-board-reporter label-sync --config config.yaml --dry-run
./bin/jira-board-reporter status-report --config config.yaml --dry-run
./bin/jira-board-reporter stale-report --config config.yaml --dry-run

# Real run
./bin/jira-board-reporter label-sync --config config.yaml
./bin/jira-board-reporter status-report --config config.yaml
./bin/jira-board-reporter stale-report --config config.yaml
```

## GitHub Actions Setup

1. Push this repo to GitHub
2. Add secrets in **Settings > Secrets and variables > Actions**:
   - `JIRA_EMAIL`
   - `JIRA_API_TOKEN`
   - `SLACK_WEBHOOK_URL`
3. Commit your `config.yaml` (it contains no secrets)
4. Workflows run on schedule:
   - **Label Sync**: Monday 06:00 UTC
   - **Status Report**: Monday 09:00 UTC
   - **Stale Report**: Wednesday 09:00 UTC
5. All workflows support manual trigger via `workflow_dispatch`

## Config

See [config.example.yaml](config.example.yaml) for all options. Key sections:

| Section | Description |
|---------|-------------|
| `jira.base_url` | Your Jira Cloud instance URL |
| `team.members` | List of Jira usernames to track |
| `board.label` | Label to add/filter by |
| `board.project_keys` | Scope label-sync to these projects |
| `board.jql_filter` | Optional custom JQL (overrides label filter) |
| `stale_thresholds` | Days without status change per issue type |
| `status_report.lookback_days` | How far back to check for changes |

### Override hierarchy

CLI flags > Environment variables > config.yaml > defaults

## Commands

```
jira-board-reporter label-sync       [--dry-run] [--config path]
jira-board-reporter status-report    [--dry-run] [--lookback-days N] [--config path]
jira-board-reporter stale-report     [--dry-run] [--epic-threshold N] [--default-threshold N] [--config path]
```

## Docker

```bash
make docker-build
docker run --rm \
  -e JIRA_EMAIL=you@example.com \
  -e JIRA_API_TOKEN=token \
  -e SLACK_WEBHOOK_URL=https://hooks.slack.com/... \
  -v $(pwd)/config.yaml:/config.yaml \
  jira-board-reporter label-sync --config /config.yaml
```
