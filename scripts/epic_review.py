#!/usr/bin/env python3
"""Dispatch a Cursor SDK agent to score and fix Epic/Feature descriptions.

Reads a JSON array of Jira issue keys from stdin and prompts a cloud agent
with the epic-style-review skill to score each issue against readiness
criteria and update descriptions that fall below threshold.

Required environment variables:
  CURSOR_API_KEY       - Cursor API key (crsr_...)
  ATLASSIAN_MCP_TOKEN  - Bearer token for Atlassian MCP server

Optional:
  CURSOR_MODEL         - Model ID (default: composer-2.5)
  EPIC_REVIEW_DRY_RUN  - Set to "true" to score without pushing updates
"""

import json
import os
import sys

from cursor_sdk import Agent, AgentOptions, CloudAgentOptions

SKILL_PROMPT = """\
You are a Jira hygiene bot. For each issue key provided, fetch its full \
details via MCP, score it against the rubric below, and if it falls below \
threshold for its current status, generate and push an improved description.

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
   b. Generate improved description covering all criteria
   c. Push via jira_update_issue
   d. Re-score to confirm it passes
5. Print a summary table for the issue

## Writing Style

- Concise. Target 40-55 lines per epic description.
- No "This epic covers..." preamble in summaries.
- AC items are one-line testable statements, not paragraphs.
- No bold lead-in phrases like "**All references removed** —" in AC.
- Approach is 2-3 sentences, not a numbered procedure.
- Do not reference the scoring rubric, style guide, or bot in generated descriptions.

## TELCOSTRAT Project Note

For TELCOSTRAT issues, do NOT update the description directly. Instead, add a \
Jira comment with the suggested improved description so the owner can paste it \
manually (the TELCOSTRAT editor does not render wiki markup).

---

{dry_run_instruction}

Review and fix the following issues: {keys}
"""


def main():
    api_key = os.environ.get("CURSOR_API_KEY")
    if not api_key:
        print("ERROR: CURSOR_API_KEY environment variable is required", file=sys.stderr)
        sys.exit(1)

    mcp_token = os.environ.get("ATLASSIAN_MCP_TOKEN", "")
    model = os.environ.get("CURSOR_MODEL", "composer-2.5")
    dry_run = os.environ.get("EPIC_REVIEW_DRY_RUN", "").lower() in ("true", "1")

    raw_input = sys.stdin.read().strip()
    if not raw_input:
        print("No input provided, nothing to review")
        sys.exit(0)

    keys = json.loads(raw_input)
    if not keys:
        print("No epics/features to review")
        sys.exit(0)

    print(f"Reviewing {len(keys)} issue(s): {', '.join(keys)}")

    dry_run_instruction = (
        "DRY RUN MODE: Score all issues and report results, but do NOT push any updates."
        if dry_run
        else ""
    )

    prompt = SKILL_PROMPT.format(
        keys=", ".join(keys),
        dry_run_instruction=dry_run_instruction,
    )

    mcp_servers = []
    if mcp_token:
        mcp_servers = [
            {
                "url": "https://mcp.atlassian.com/v1/mcp",
                "headers": {"Authorization": mcp_token},
            }
        ]

    result = Agent.prompt(
        prompt,
        AgentOptions(
            api_key=api_key,
            model=model,
            cloud=CloudAgentOptions(
                repos=[{"owner": "yaronh12", "name": "jira-board-keeper"}],
            ),
            mcp_servers=mcp_servers if mcp_servers else None,
        ),
    )

    print(f"\nAgent status: {result.status}")
    if result.result:
        print(result.result)

    if result.status == "error":
        sys.exit(2)


if __name__ == "__main__":
    main()
