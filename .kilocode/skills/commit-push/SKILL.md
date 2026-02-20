---
description: Git commit and push with conventional commit format. No AI attribution.
---

**INSTRUCTION:** Execute commit and push directly (no subagent).

**CRITICAL RULES:**
- NO AI attribution (Claude, AI, automated, assistant, co-author)
- Conventional commit format: `type(scope): description`
- AUTO-CONFIRM all git operations

**Workflow:**

1. **Setup & Stage** (single chained command):
   ```bash
   git config user.name "bobmcallan" && git config --global credential.github.com.username bobmcallan && git config core.autocrlf input && git add .
   ```

2. **Analyze** (parallel calls):
   - `git branch --show-current`
   - `git diff --cached --stat`
   - `git log --oneline -5` (for commit style reference)
   - **Format & lint** — detect project type and run the appropriate formatter, then re-stage:
     - `go.mod` exists → `gofmt -s -w . && git add -u`
     - `*.tf` files exist → `terraform fmt -recursive && git add -u`
     - Multiple may apply — run all that match

3. **Bump version** — if `.version` exists, bump the patch number:
   - Read current version (e.g. `version: 0.2.0`)
   - Increment patch: `0.2.0` → `0.2.1`
   - Update build timestamp: `build: MM-DD-HH-MM-SS`
   - Write back to `.version` and `git add .version`

4. **Commit & Push** (single chained command):
   ```bash
   git commit -m "type(scope): message" && git push
   ```

**Commit Types:** feat | fix | docs | refactor | test | chore

**Output:** Commit hash, message, push status.

**Context:** $ARGUMENTS
