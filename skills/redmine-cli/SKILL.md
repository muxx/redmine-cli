---
name: redmine-cli
description: Use when working with Redmine through the redmine command-line tool: configuring authentication profiles, checking auth, listing/showing/creating/updating Redmine issues, projects, users, trackers, statuses, and discovering command syntax via help.
---

# Redmine CLI

Use the `redmine` command for Redmine API work. Prefer it over raw REST calls or `curl`; fall back to raw HTTP only when the CLI cannot perform the action and the user explicitly approves.

## Setup

- Check the tool first: `redmine --version`.
- If it is missing, ask the user to install it from Homebrew or release artifacts.
- Use named profiles for every persistent Redmine connection.
- Configure a profile:

```bash
redmine auth login --profile <name> --host <redmine-url> --api-key <api-key>
```

- If the key is in an environment variable, avoid placing it in process arguments:

```bash
printf '%s' "$REDMINE_API_KEY" | redmine auth login --profile <name> --host <redmine-url> --stdin
```

- Check auth before doing work: `redmine --profile <name> auth status`.
- Never echo API keys, passwords, or config file contents.

## Profile Selection

- Use `--profile <name>` on commands when the target Redmine is known.
- If the user has a current profile configured and did not ask for another one, plain `redmine ...` is acceptable.
- Use `redmine auth list` to see saved profiles and `redmine auth use <name>` to switch the current profile.
- Profile resolution order is `--profile`, `REDMINE_PROFILE`, current profile from config, then `default`.

## Command Discovery

When unsure about syntax, inspect help instead of guessing:

```bash
redmine --help
redmine issue --help
redmine issue update --help
```

Use `redmine --help` and subcommand help as the primary command reference. If the `redmine-cli` repository is available, `docs/usage.md` contains the generated full command reference.

## Common Commands

```bash
redmine --profile <name> user show-current
redmine --profile <name> project list
redmine --profile <name> tracker list
redmine --profile <name> issue-status list
redmine --profile <name> issue show <issue_id> --include journals
redmine --profile <name> issue list --project-id <project_id> --assigned-to-id me --status-id o
redmine --profile <name> issue create --project-id <project_id> --tracker-id <tracker_id> --subject "<title>" --description "<text>"
```

For complex creates or updates, prefer a JSON body file so field names and nested values stay explicit:

```bash
redmine --profile <name> issue update <issue_id> --body @issue-update.json
```

```json
{
  "issue": {
    "status_id": 3,
    "done_ratio": 100,
    "notes": "Update text"
  }
}
```

## Working Rules

- Do not create or update issues when required project-specific fields are unknown; inspect Redmine data or ask targeted questions.
- Include direct Redmine links in the final response when the host and issue IDs are known.
- Preserve the server or project text formatting convention for descriptions and notes. If unknown, use plain text.
