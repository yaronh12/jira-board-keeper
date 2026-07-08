#!/usr/bin/env python3
"""Score Epic/Feature descriptions, create GitHub Issues for review, notify via Slack.

Reads a JSON array of Jira issue keys from stdin, dispatches a Cursor SDK agent
to score each issue, then for each failing issue:
  1. Creates a GitHub Issue with the suggested improved description
  2. Posts to the team Slack channel notifying the assignee

The assignee reviews the GitHub Issue and comments "/approve" to trigger
the description update in Jira.

Required environment variables:
  CURSOR_API_KEY       - Cursor API key (crsr_...)
  GITHUB_TOKEN         - GitHub token for creating issues (auto-provided in Actions)
  GITHUB_REPOSITORY    - owner/repo (auto-provided in Actions)

Optional:
  ATLASSIAN_MCP_TOKEN  - Auth header for Atlassian MCP server
  SLACK_WEBHOOK_URL    - Slack incoming webhook for team channel notifications
  CURSOR_MODEL         - Model ID (default: composer-2.5)
  EPIC_REVIEW_DRY_RUN  - Set to "true" to skip creating issues/posting (print only)
  JIRA_BASE_URL        - Jira instance URL (default: https://redhat.atlassian.net)
"""

import json
import os
import re
import sys

import requests
import yaml
from cursor_sdk import Agent, AgentOptions, LocalAgentOptions

SKILL_PROMPT = """\
You are a Jira hygiene bot. For each issue key provided, fetch its full \
details via MCP, score it against the rubric below, and report which issues \
fail their threshold along with a suggested improved description.

IMPORTANT: Do NOT update any Jira issues. Only score and suggest.

---

## Feature Scoring (S1-S11, /13)

| ID | Condition | Pts |
|----|-----------|-----|
| S1 | Summary describes what, who, why (2-4 sentences, self-contained) | 1 |
| S2 | Business justification states the business problem (BLOCKER if 0) | 2 |
| S3 | At least 1 concrete use case with topology/context | 1 |
| S4 | At least 2 specific, outcome-oriented goals | 1 |
| S5 | 3+ testable acceptance criteria | 2 |
| S6 | Definition of done stated explicitly | 1 |
| S7 | Out of scope defined with at least 1 boundary | 1 |
| S8 | Functional requirements listed | 1 |
| S9 | Non-functional requirements with concrete numbers | 1 |
| S10 | Test expectations described | 1 |
| S11 | No section is just a link to an external document | 1 |

### Feature Thresholds

| Status | Min Score |
|--------|-----------|
| New | 4/13 |
| Refinement | 9/13 |
| In Progress (Dev Preview) | 9/13 |
| In Progress (Tech Preview) | 11/13 |
| In Progress (GA) | 12/13 |

S2 = 0 is a blocker regardless of total score.

## Epic Scoring (E1-E10, /12)

| ID | Condition | Pts |
|----|-----------|-----|
| E1 | Summary describes what ships in this release (1-3 sentences) | 1 |
| E2 | Parent feature linked (or KTLO justification) | 1 |
| E3 | Release scope explicit (what ships vs deferred) | 1 |
| E4 | 3+ testable acceptance criteria | 2 |
| E5 | Definition of done stated | 1 |
| E6 | Implementation approach described | 1 |
| E7 | Dependencies identified (upstream, cross-team, hardware) | 1 |
| E8 | Test plan: QE needed stated (yes/no), dev CI described | 1 |
| E9 | Can be broken into stories/tasks (candidates listed) | 2 |
| E10 | No section is just a link to an external document | 1 |

### Epic Thresholds

| Status | Min Score |
|--------|-----------|
| New | 4/12 |
| Refinement | 8/12 |
| In Progress | 10/12 |

### E4 and E9 Scoring Detail

- E4: 0 = no AC or boilerplate; 1 = 1-2 testable AC; 2 = 3+ testable AC
- E9: 0 = no breakdown; 1 = tasks implied but not listed; 2 = explicit candidate stories/tasks

---

## Workflow

For each issue key:

1. Fetch via jira_get_issue with fields=*all
2. Determine type (Feature vs Epic) from issue_type
3. Score each criterion
4. If below threshold for current status:
   a. Pull child stories via jira_search (parent = KEY OR "Epic Link" = KEY)
   b. Generate an improved description covering all criteria

## Writing Style for Suggested Descriptions

- Concise. Target 40-55 lines per epic description.
- No "This epic covers..." preamble in summaries.
- AC items are one-line testable statements, not paragraphs.
- No bold lead-in phrases like "**All references removed** —" in AC.
- Approach is 2-3 sentences, not a numbered procedure.
- Do not reference the scoring rubric, style guide, or bot in generated descriptions.

---

## Output Format

You MUST output ONLY a JSON array. No markdown, no commentary, no code fences.
Include one object per issue that FAILS its threshold. If all pass, output `[]`.

Each object must have these fields:
- "key": the Jira issue key (string)
- "assignee_name": the assignee's display name from Jira (string, or "" if unassigned)
- "assignee_email": the assignee's email from Jira (string, or "" if unassigned)
- "summary": the issue summary/title (string)
- "score": e.g. "7/12" (string)
- "threshold": e.g. "10/12" (string)
- "gaps": array of strings, each describing a missing criterion (e.g. "E5: No Definition of Done")
- "suggested_description": the full improved description text (string)

---

Review the following issues: {keys}
"""


def extract_json_from_result(result_text):
    """Extract JSON array from agent output, handling possible markdown fences."""
    text = result_text.strip()
    fence_match = re.search(r"```(?:json)?\s*\n?(.*?)\n?```", text, re.DOTALL)
    if fence_match:
        text = fence_match.group(1).strip()
    bracket_match = re.search(r"\[.*\]", text, re.DOTALL)
    if bracket_match:
        text = bracket_match.group(0)
    return json.loads(text)


def create_github_issue(token, repo, issue, github_usernames=None):
    """Create a GitHub Issue for a failing epic with the suggested description."""
    key = issue["key"]
    score = issue["score"]
    threshold = issue["threshold"]
    gaps = issue["gaps"]
    suggestion = issue["suggested_description"]
    assignee_name = issue.get("assignee_name", "Unassigned")
    jira_base_url = os.environ.get("JIRA_BASE_URL", "https://redhat.atlassian.net").rstrip("/")

    title = f"Epic Review: {key} — {score} (need {threshold})"

    body = f"""## {key}: {issue.get('summary', '')}

**Jira:** {jira_base_url}/browse/{key}
**Assignee:** {assignee_name}
**Score:** {score} (threshold: {threshold})

### Missing Criteria

{chr(10).join(f'- {g}' for g in gaps)}

### Suggested Description

To approve this change, comment `/approve` on this issue.

```
{suggestion}
```

---
*Generated by epic-style-review bot. Comment `/approve` to push this description to Jira.*
"""

    payload = {
        "title": title,
        "body": body,
        "labels": ["epic-review"],
    }

    gh_username = (github_usernames or {}).get(assignee_name, "")
    if gh_username:
        payload["assignees"] = [gh_username]

    resp = requests.post(
        f"https://api.github.com/repos/{repo}/issues",
        headers={
            "Authorization": f"Bearer {token}",
            "Accept": "application/vnd.github+json",
        },
        json=payload,
        timeout=15,
    )

    if resp.status_code == 201:
        return resp.json()["html_url"]
    print(f"  Failed to create issue for {key}: {resp.status_code} {resp.text}", file=sys.stderr)
    return None


def post_slack_notification(webhook_url, issue, github_issue_url, jira_base_url):
    """Post a Slack message to the team channel about a failing epic."""
    key = issue["key"]
    score = issue["score"]
    threshold = issue["threshold"]
    assignee = issue.get("assignee_name", "Unassigned")
    summary = issue.get("summary", "")
    jira_url = f"{jira_base_url}/browse/{key}"

    blocks = [
        {
            "type": "section",
            "text": {
                "type": "mrkdwn",
                "text": (
                    f":clipboard: *Epic Review: <{jira_url}|{key}>*\n"
                    f">{summary}\n\n"
                    f"*Score:* {score} (need {threshold}) — *BELOW THRESHOLD*\n"
                    f"*Assignee:* {assignee}\n"
                    f"*Review & approve:* <{github_issue_url}|GitHub Issue>"
                ),
            },
        },
    ]

    fallback = f"Epic Review: {key} scored {score} (need {threshold}) — {assignee} please review: {github_issue_url}"

    resp = requests.post(
        webhook_url,
        json={"text": fallback, "blocks": blocks},
        timeout=10,
    )
    return resp.status_code == 200


def main():
    api_key = os.environ.get("CURSOR_API_KEY")
    if not api_key:
        print("ERROR: CURSOR_API_KEY environment variable is required", file=sys.stderr)
        sys.exit(1)

    github_token = os.environ.get("GITHUB_TOKEN", "")
    github_repo = os.environ.get("GITHUB_REPOSITORY", "")
    slack_webhook = os.environ.get("SLACK_WEBHOOK_URL", "")
    mcp_token = os.environ.get("ATLASSIAN_MCP_TOKEN", "")
    model = os.environ.get("CURSOR_MODEL", "composer-2.5")
    dry_run = os.environ.get("EPIC_REVIEW_DRY_RUN", "").lower() in ("true", "1")
    jira_base_url = os.environ.get("JIRA_BASE_URL", "https://redhat.atlassian.net").rstrip("/")

    github_usernames = {}
    config_path = os.path.join(os.getcwd(), "config.yaml")
    if os.path.exists(config_path):
        with open(config_path) as f:
            config = yaml.safe_load(f)
        github_usernames = config.get("team", {}).get("members", {})

    raw_input = sys.stdin.read().strip()
    if not raw_input:
        print("No input provided, nothing to review")
        sys.exit(0)

    keys = json.loads(raw_input)
    if not keys:
        print("No epics/features to review")
        sys.exit(0)

    print(f"Reviewing {len(keys)} issue(s): {', '.join(keys)}")

    prompt = SKILL_PROMPT.format(keys=", ".join(keys))

    mcp_servers = None
    if mcp_token:
        mcp_servers = {
            "atlassian": {
                "url": "https://mcp.atlassian.com/v1/mcp",
                "headers": {"Authorization": mcp_token},
            }
        }

    result = Agent.prompt(
        prompt,
        AgentOptions(
            api_key=api_key,
            model=model,
            local=LocalAgentOptions(cwd=os.getcwd()),
            mcp_servers=mcp_servers if mcp_servers else None,
        ),
    )

    print(f"\nAgent status: {result.status}")

    if result.status == "error":
        print(f"Agent error: {result.result}", file=sys.stderr)
        sys.exit(2)

    if not result.result:
        print("Agent returned no output")
        sys.exit(0)

    try:
        failing_issues = extract_json_from_result(result.result)
    except (json.JSONDecodeError, TypeError) as e:
        print(f"Could not parse agent JSON output: {e}", file=sys.stderr)
        print(f"Raw output:\n{result.result}")
        sys.exit(2)

    if not failing_issues:
        print("All issues pass their threshold. Nothing to do.")
        sys.exit(0)

    print(f"\n{len(failing_issues)} issue(s) below threshold:")
    for issue in failing_issues:
        print(f"  {issue['key']}: {issue['score']} (need {issue['threshold']})")

    if dry_run:
        print("\nDRY RUN — would create GitHub Issues and notify Slack for:")
        for issue in failing_issues:
            print(f"  {issue['key']} (assignee: {issue.get('assignee_name', 'unknown')})")
        sys.exit(0)

    if not github_token or not github_repo:
        print("\nGITHUB_TOKEN or GITHUB_REPOSITORY not set — cannot create issues")
        print("Results:")
        for issue in failing_issues:
            print(f"\n--- {issue['key']} ({issue['score']}) ---")
            print(f"Gaps: {', '.join(issue['gaps'])}")
            print(f"Suggestion:\n{issue['suggested_description'][:500]}...")
        sys.exit(1)

    created = 0
    notified = 0
    for issue in failing_issues:
        gh_url = create_github_issue(github_token, github_repo, issue, github_usernames)
        if gh_url:
            print(f"  {issue['key']}: created {gh_url}")
            created += 1

            if slack_webhook:
                ok = post_slack_notification(slack_webhook, issue, gh_url, jira_base_url)
                if ok:
                    notified += 1
                else:
                    print(f"  {issue['key']}: Slack notification failed")
        else:
            print(f"  {issue['key']}: failed to create GitHub issue")

    print(f"\nDone. Issues created: {created}, Slack notifications: {notified}")


if __name__ == "__main__":
    main()
