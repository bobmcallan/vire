# /deploy - Build and Deploy Vire

Build a local Docker image, deploy containers, and validate health.

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

### Step 2: Verify Local Image

Before building, check the running container is using a **local** image, not a remote/ghcr image:

```bash
docker ps --filter "name=vire-server" --format "{{.Image}}"
```

- If the image is `vire-server:latest` — local image, proceed normally.
- If the image is `ghcr.io/bobmcallan/vire-server:*` or any remote image — the container was started from `docker-compose.ghcr.yml` instead of the local compose file. **Force a rebuild** regardless of the change assessment to replace the remote container with a locally built one.

### Step 3: Build and Deploy

The default is always **local build**. Run:

```bash
./scripts/deploy.sh local [--force]
```

Use `--force` when:
- User passed `--force`
- Dockerfile or docker-compose.yml changed
- go.mod/go.sum changed (dependency update)
- The running container uses a remote image (see Step 2)

The script handles:
1. Stopping any ghcr container (`docker-compose.ghcr.yml down`)
2. Building the local image (`docker-compose.yml build`)
3. Starting/recreating the container (`docker-compose.yml up -d --force-recreate`)
4. Touching `docker/.last_build` marker

### Step 4: Validate

After deploy, wait 30 seconds for the container to start, then run validation:

1. **Container image** — confirm the image is local, not remote:
   ```bash
   docker ps --filter "name=vire-server" --format "table {{.Names}}\t{{.Image}}\t{{.Status}}"
   ```
   The image column MUST show `vire-server:latest`, NOT `ghcr.io/...`. If it shows a remote image, the deploy did not work correctly — report and stop.

2. **Container health** — confirm healthy status:
   ```bash
   docker ps --filter "name=vire" --format "table {{.Names}}\t{{.Image}}\t{{.Status}}"
   ```

3. **Health endpoint** — confirm the server is responding:
   ```bash
   curl -sf http://localhost:4242/api/health
   ```

4. **Version check** — confirm deployed version matches `.version`:
   ```bash
   curl -sf http://localhost:4242/api/health | jq -r '.version // empty'
   ```

5. **Test suite** (unless `--skip-tests`):
   ```bash
   go test ./...
   go vet ./...
   ```

6. **Report results:**
   - Container image (local vs remote)
   - Container status (running/healthy/unhealthy)
   - Deployed version
   - Test pass/fail count
   - Any errors or warnings

### Step 5: On Failure

If deploy or validation fails:
- Show the error output
- Check `docker logs vire-server` for startup errors
- If the container is still using a remote image after local deploy, the ghcr compose may not have been stopped. Try: `docker compose -f docker/docker-compose.ghcr.yml down && ./scripts/deploy.sh local --force`
- Suggest fix based on the error (build failure vs runtime failure vs test failure)
- Do NOT automatically retry — report and let the user decide

## Context
$ARGUMENTS
