# Scripts

## epic_review.py

Dispatches a Cursor SDK cloud agent to score and improve Epic/Feature
descriptions using the epic-style-review rubric.

### Setup

```bash
pip install -r scripts/requirements.txt
```

### Required secrets (GitHub Actions)

| Secret | How to obtain |
|--------|---------------|
| `CURSOR_API_KEY` | [Cursor Dashboard → API](https://cursor.com/dashboard/api) → create a key with Admin scope |
| `ATLASSIAN_MCP_TOKEN` | See "Atlassian MCP Auth" below |

### Atlassian MCP Auth

The Atlassian MCP server at `mcp.atlassian.com` uses OAuth 2.0. For CI:

**Option A: OAuth2 access token (recommended for enterprise)**

1. Create an OAuth 2.0 (3LO) app at [developer.atlassian.com](https://developer.atlassian.com/console/myapps/)
2. Grant scopes: `read:jira-work`, `write:jira-work`
3. Complete the OAuth flow once to obtain a refresh token
4. Store the refresh token as `ATLASSIAN_MCP_REFRESH_TOKEN`
5. Add a workflow step to exchange it for an access token before running the script

**Option B: Use Cursor plugin auth (simplest)**

If your Cursor enterprise team has the Atlassian plugin configured at the
team/org level, cloud agents inherit that auth automatically. In this case,
you can remove the `mcp_servers` override in `epic_review.py` and the agent
will use the team-configured Atlassian plugin directly.

To test this, set `ATLASSIAN_MCP_TOKEN` to an empty string — the script will
skip the explicit MCP server config and rely on plugin-level auth.

### Local testing

```bash
echo '["CNF-23509"]' | CURSOR_API_KEY=crsr_... ATLASSIAN_MCP_TOKEN=... python scripts/epic_review.py
```

With dry-run (score only, no updates):

```bash
echo '["CNF-23509"]' | CURSOR_API_KEY=crsr_... ATLASSIAN_MCP_TOKEN=... EPIC_REVIEW_DRY_RUN=true python scripts/epic_review.py
```
