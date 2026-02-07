# /strategy - Portfolio Strategy Management

Create, view, and update the investment strategy for a portfolio. The strategy drives portfolio reviews, market scans, and stock screening by filtering and scoring results based on your risk appetite, sector preferences, and return targets.

## Usage
```
/strategy [portfolio_name] [action] [options]
```

**Actions:**
- `view` (default) - View current strategy
- `build` - Interactive strategy-building conversation
- `update` - Update specific strategy fields
- `delete` - Delete the strategy
- `template` - Show all available strategy fields

**Examples:**
- `/strategy SMSF` - View SMSF strategy
- `/strategy SMSF build` - Start interactive strategy builder for SMSF
- `/strategy Personal update` - Update Personal strategy fields
- `/strategy template` - Show strategy field reference

## Prerequisites - Auto Build & Start

Before executing the workflow, ensure the MCP server is running with latest code:

### Step 0: Check and Rebuild Container
```bash
cd /home/bobmc/development/vire

# Check if source files are newer than last build
NEEDS_REBUILD=false
if [ ! -f docker/.last_build ]; then
    NEEDS_REBUILD=true
else
    if find . -name "*.go" -newer docker/.last_build 2>/dev/null | grep -q . || \
       [ go.mod -nt docker/.last_build ] || [ go.sum -nt docker/.last_build ]; then
        NEEDS_REBUILD=true
    fi
fi

if [ "$NEEDS_REBUILD" = true ]; then
    echo "Code changes detected, rebuilding container..."
    docker compose -f docker/docker-compose.yml build
    touch docker/.last_build
    docker compose -f docker/docker-compose.yml up -d
else
    if ! docker compose -f docker/docker-compose.yml ps --status running | grep -q vire-mcp; then
        docker compose -f docker/docker-compose.yml up -d
    fi
fi

sleep 2
```

Run this bash script before proceeding with the MCP workflow steps.

## Workflow: View Strategy

```
Use: get_portfolio_strategy
Parameters:
  - portfolio_name: {portfolio_name}
```

Display the returned markdown to the user.

## Workflow: Build Strategy (Interactive)

Guide the user through strategy creation in a conversational flow.

### Step 1: Get template
```
Use: get_strategy_template
Parameters:
  - account_type: {smsf or trading, ask user first}
```

### Step 2: Ask questions conversationally

Walk through these topics one at a time (do NOT dump all fields at once):

1. **Account type** - SMSF or standard trading account?
2. **Risk appetite** - Conservative, moderate, or aggressive? What maximum drawdown can you tolerate?
3. **Target returns** - What annual return are you targeting? Over what timeframe?
4. **Income requirements** - Do you need dividend income? What yield target?
5. **Investment universe** - Which exchanges? (AU, US, etc.)
6. **Sector preferences** - Any sectors you prefer or want to exclude?
7. **Position sizing** - Maximum single position %? Maximum sector allocation %?
8. **Reference strategies** - Any named strategies to follow? (value investing, dividend growth, etc.)
9. **Rebalance frequency** - How often do you review? (monthly, quarterly, annually)

### Step 3: Save strategy
Build the JSON from user answers and save:
```
Use: set_portfolio_strategy
Parameters:
  - portfolio_name: {portfolio_name}
  - strategy_json: {JSON object with user's answers}
```

### Step 4: Present warnings
The response includes devil's advocate warnings. Present each warning to the user and discuss. If they want to adjust, go back to Step 3 with updated fields.

### Step 5: Confirm
Show the final strategy markdown and confirm the user is happy.

## Workflow: Update Strategy

```
Use: set_portfolio_strategy
Parameters:
  - portfolio_name: {portfolio_name}
  - strategy_json: {JSON with ONLY the fields to change}
```

Uses merge semantics: only include fields you want to change. Unspecified fields keep their current values.

## Workflow: Delete Strategy

Confirm with the user first, then:
```
Use: delete_portfolio_strategy
Parameters:
  - portfolio_name: {portfolio_name}
```

## Strategy Impact

Once set, the strategy automatically influences:

| Feature | How Strategy Applies |
|---------|---------------------|
| Portfolio Review | RSI/SMA thresholds adjusted by risk level; position size alerts; strategy-specific recommendations |
| Market Snipe | Excluded sectors filtered out; conservative strategies penalise volatile candidates; risk appetite in AI prompt |
| Stock Screen | Default P/E threshold adjusted (conservative=15, aggressive=25); dividend scoring boosted for conservative; target returns in AI prompt |

**Precedence rule:** Explicit user parameters always override strategy defaults. For example, passing `sector=Mining` to market_snipe will include Mining even if the strategy excludes it.

## Response Guidelines

- When building a strategy, be conversational and explain trade-offs
- Present devil's advocate warnings seriously -- they flag real contradictions
- For SMSF accounts, highlight regulatory considerations
- Never suggest specific investments -- the strategy is a planning document
- Always remind users this is not financial advice
