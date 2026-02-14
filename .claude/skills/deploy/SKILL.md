# /deploy - Build and Deploy Vire

Assess code changes, rebuild Docker images, deploy containers, and validate health.

## Usage
```
/deploy [options]
```

## Options
- `--force` — Force full rebuild (no cache)
- `--skip-tests` — Skip test suite after deploy
## Examples
- `/deploy` — Smart rebuild and deploy
- `/deploy --force` — Full rebuild from scratch

## Procedure

### Step 1: Assess Changes

Determine what changed since the last build:

```bash
# Check what files changed since last build marker
if [ -f docker/.last_build ]; then
  find . -name "*.go" -newer docker/.last_build
else
  echo "No previous build marker — full rebuild needed"
fi
```

Categorise changes into:
- **Server only** — files under `cmd/vire-server/` only
- **Shared** — files under `internal/`, `go.mod`, `go.sum`
- **None** — no `.go` file changes (skip rebuild unless `--force`)

Report the assessment before proceeding.

### Step 2: Build and Deploy

Run the deploy script:

```bash
./scripts/deploy.sh local [--force]
```

Use `--force` when:
- User passed `--force`
- Dockerfile or docker-compose.yml changed
- go.mod/go.sum changed (dependency update)

### Step 3: Validate

After deploy, run validation checks:

1. **Container health** — wait for containers to be healthy:
   ```bash
   docker ps --filter "name=vire" --format "table {{.Names}}\t{{.Image}}\t{{.Status}}"
   ```

2. **Health endpoint** — confirm the server is responding:
   ```bash
   curl -sf http://localhost:4242/api/health
   ```

3. **Version check** — confirm deployed version matches `.version`:
   ```bash
   curl -sf http://localhost:4242/api/health | jq -r '.version // empty'
   ```

4. **Test suite** (unless `--skip-tests`):
   ```bash
   go test ./...
   go vet ./...
   ```

5. **Report results:**
   - Container status (running/healthy/unhealthy)
   - Deployed version
   - Test pass/fail count
   - Any errors or warnings

### Step 4: On Failure

If deploy or validation fails:
- Show the error output
- Check `docker logs vire-server` for startup errors
- Suggest fix based on the error (build failure vs runtime failure vs test failure)
- Do NOT automatically retry — report and let the user decide

## Context
$ARGUMENTS
