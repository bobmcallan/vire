# /execute - Script Runner with Auto-Fix

Execute a script and automatically investigate, fix, and retry if it fails.

## Usage

/execute <script-path> [options]

## Options
- `--max-retries N` — Maximum fix-and-retry attempts (default: 5)

## Examples
- `/execute ./scripts/deploy.sh local`
- `/execute ./scripts/build.sh --clean`
- `/execute ./tests/run_integration.sh --max-retries 3`

## Workflow

### Step 1: Validate Script
- Confirm the script file exists and is executable
- If not executable, run `chmod +x` on it
- Read the script to understand what it does

### Step 2: Execute Script
- Run the script with any provided arguments
- Capture both stdout and stderr
- Record the exit code

### Step 3: On Success
- Report: script path, exit code, and key output lines
- Done

### Step 4: On Failure — Investigate & Fix Loop
If the script exits non-zero, iterate up to max-retries times:

1. **Analyse the error** — Read stderr/stdout, identify the root cause
2. **Investigate** — Read relevant source files, configs, or logs referenced in the error
3. **Fix** — Apply the fix (edit the script, fix source code, install missing dependency, etc.)
4. **Re-run** — Execute the script again
5. **If still failing** — Go to step 1 with the new error context

### Step 5: Give Up
If max retries exhausted:
- Report all attempted fixes and their outcomes
- Show the final error output
- Suggest what to try next

## Guidelines
- Always read the script before first execution to understand context
- Prefer minimal, targeted fixes — don't refactor unrelated code
- If the error is in source code (not the script itself), fix the source
- If the error requires user input (credentials, env vars, choices), ask rather than guess
- Track each iteration: what failed, what was fixed, outcome
