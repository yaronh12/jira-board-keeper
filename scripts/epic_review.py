#!/usr/bin/env python3
"""Score Epic/Feature descriptions and DM assignees with suggested improvements.

Reads a JSON array of Jira issue keys from stdin, dispatches a Cursor SDK agent
to score each issue, then sends a Slack DM to assignees whose epics fail the
threshold — including the score, gaps, and a suggested improved description.

Required environment variables:
  CURSOR_API_KEY       - Cursor API key (crsr_...)
  SLACK_BOT_TOKEN      - Slack bot token (xoxb-...) with chat:write, users:read.email

Optional:
  ATLASSIAN_MCP_TOKEN  - Auth header for Atlassian MCP server
  CURSOR_MODEL         - Model ID (default: composer-2.5)
  EPIC_REVIEW_DRY_RUN  - Set to "true" to skip sending DMs (print only)
  JIRA_BASE_URL        - Jira instance URL (default: https://redhat.atlassian.net)
"""

import json
import os
import re
import sys

import requests
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
- "assignee_email": the assignee's email from Jira (string, or "" if unassigned)
- "summary": the issue summary/title (string)
- "score": e.g. "7/12" (string)
- "threshold": e.g. "10/12" (string)
- "gaps": array of strings, each describing a missing criterion (e.g. "E5: No Definition of Done")
- "suggested_description": the full improved description text (string)

---

Review the following issues: {keys}
"""


def lookup_slack_user(bot_token, email):
    """Resolve a Slack user ID from their email address."""
    resp = requests.get(
        "https://slack.com/api/users.lookupByEmail",
        headers={"Authorization": f"Bearer {bot_token}"},
        params={"email": email},
        timeout=10,
    )
    data = resp.json()
    if data.get("ok"):
        return data["user"]["id"]
    return None


def send_slack_dm(bot_token, user_id, blocks, fallback_text):
    """Send a Block Kit DM to a Slack user."""
    resp = requests.post(
        "https://slack.com/api/chat.postMessage",
        headers={"Authorization": f"Bearer {bot_token}"},
        json={"channel": user_id, "text": fallback_text, "blocks": blocks},
        timeout=10,
    )
    return resp.json().get("ok", False)


def format_dm_blocks(issue, jira_base_url):
    """Build Slack Block Kit blocks for a review DM."""
    key = issue["key"]
    summary = issue.get("summary", "")
    score = issue["score"]
    threshold = issue["threshold"]
    gaps = issue["gaps"]
    suggestion = issue["suggested_description"]

    issue_url = f"{jira_base_url}/browse/{key}"

    blocks = [
        {
            "type": "header",
            "text": {"type": "plain_text", "text": f"Epic Review: {key}"},
        },
        {
            "type": "section",
            "text": {
                "type": "mrkdwn",
                "text": (
                    f"*<{issue_url}|{key}>* {summary}\n"
                    f"Score: *{score}* (need {threshold}) — *BELOW THRESHOLD*"
                ),
            },
        },
        {"type": "divider"},
        {
            "type": "section",
            "text": {
                "type": "mrkdwn",
                "text": "*Missing criteria:*\n" + "\n".join(f"• {g}" for g in gaps),
            },
        },
        {"type": "divider"},
        {
            "type": "section",
            "text": {
                "type": "mrkdwn",
                "text": "*Suggested description:*",
            },
        },
    ]

    # Slack has a 3000 char limit per text block — split if needed
    desc_chunks = split_text(suggestion, 2900)
    for chunk in desc_chunks:
        blocks.append(
            {
                "type": "section",
                "text": {"type": "mrkdwn", "text": f"```{chunk}```"},
            }
        )

    blocks.append(
        {
            "type": "context",
            "elements": [
                {
                    "type": "mrkdwn",
                    "text": "This is a suggestion from the epic hygiene bot. "
                    "Copy and paste the description into the Jira issue if it looks good.",
                }
            ],
        }
    )

    return blocks


def split_text(text, max_len):
    """Split text into chunks that fit within max_len."""
    if len(text) <= max_len:
        return [text]
    chunks = []
    while text:
        chunks.append(text[:max_len])
        text = text[max_len:]
    return chunks


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


def main():
    api_key = os.environ.get("CURSOR_API_KEY")
    if not api_key:
        print("ERROR: CURSOR_API_KEY environment variable is required", file=sys.stderr)
        sys.exit(1)

    slack_token = os.environ.get("SLACK_BOT_TOKEN", "")
    mcp_token = os.environ.get("ATLASSIAN_MCP_TOKEN", "")
    model = os.environ.get("CURSOR_MODEL", "composer-2.5")
    dry_run = os.environ.get("EPIC_REVIEW_DRY_RUN", "").lower() in ("true", "1")
    jira_base_url = os.environ.get("JIRA_BASE_URL", "https://redhat.atlassian.net").rstrip("/")
    dm_override = os.environ.get("DM_OVERRIDE_EMAIL", "")

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

    # Parse structured JSON from agent output
    try:
        failing_issues = extract_json_from_result(result.result)
    except (json.JSONDecodeError, TypeError) as e:
        print(f"Could not parse agent JSON output: {e}", file=sys.stderr)
        print(f"Raw output:\n{result.result}")
        sys.exit(2)

    if not failing_issues:
        print("All issues pass their threshold. No DMs to send.")
        sys.exit(0)

    print(f"\n{len(failing_issues)} issue(s) below threshold:")
    for issue in failing_issues:
        print(f"  {issue['key']}: {issue['score']} (need {issue['threshold']})")

    if not slack_token:
        print("\nSLACK_BOT_TOKEN not set — printing suggestions only (no DMs sent)")
        for issue in failing_issues:
            print(f"\n--- {issue['key']} ({issue['score']}) ---")
            print(f"Assignee: {issue.get('assignee_email', 'unknown')}")
            print(f"Gaps: {', '.join(issue['gaps'])}")
            print(f"Suggestion:\n{issue['suggested_description'][:500]}...")
        sys.exit(0)

    if dry_run:
        print("\nDRY RUN — would send DMs to:")
        for issue in failing_issues:
            print(f"  {issue.get('assignee_email', 'unassigned')} for {issue['key']}")
        sys.exit(0)

    # Send Slack DMs
    if dm_override:
        print(f"\nDM_OVERRIDE_EMAIL set — all DMs will go to: {dm_override}")

    sent = 0
    skipped = 0
    for issue in failing_issues:
        email = issue.get("assignee_email", "")
        target_email = dm_override if dm_override else email
        if not target_email:
            print(f"  {issue['key']}: no assignee email, skipping DM")
            skipped += 1
            continue

        user_id = lookup_slack_user(slack_token, target_email)
        if not user_id:
            print(f"  {issue['key']}: could not find Slack user for {target_email}, skipping")
            skipped += 1
            continue

        blocks = format_dm_blocks(issue, jira_base_url)
        fallback = f"Epic Review: {issue['key']} scored {issue['score']} (need {issue['threshold']})"

        ok = send_slack_dm(slack_token, user_id, blocks, fallback)
        if ok:
            print(f"  {issue['key']}: DM sent to {target_email}" +
                  (f" (on behalf of {email})" if dm_override and email != dm_override else ""))
            sent += 1
        else:
            print(f"  {issue['key']}: failed to send DM to {target_email}")
            skipped += 1

    print(f"\nDone. Sent: {sent}, Skipped: {skipped}")


if __name__ == "__main__":
    main()
