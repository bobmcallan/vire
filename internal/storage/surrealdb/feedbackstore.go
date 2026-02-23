package surrealdb

import (
	"context"
	"fmt"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/google/uuid"
	"github.com/surrealdb/surrealdb.go"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// feedbackSelectFields lists the fields to select from mcp_feedback, aliasing feedback_id to id for struct mapping.
const feedbackSelectFields = `feedback_id as id, session_id, client_type, category, severity, description,
	ticker, portfolio_name, tool_name, observed_value, expected_value,
	status, resolution_notes, created_at, updated_at`

// FeedbackStore implements interfaces.FeedbackStore using SurrealDB.
type FeedbackStore struct {
	db     *surrealdb.DB
	logger *common.Logger
}

// NewFeedbackStore creates a new FeedbackStore.
func NewFeedbackStore(db *surrealdb.DB, logger *common.Logger) *FeedbackStore {
	return &FeedbackStore{db: db, logger: logger}
}

func (s *FeedbackStore) Create(ctx context.Context, fb *models.Feedback) error {
	if fb.ID == "" {
		fb.ID = fmt.Sprintf("fb_%s", uuid.New().String()[:8])
	}
	now := time.Now()
	if fb.CreatedAt.IsZero() {
		fb.CreatedAt = now
	}
	fb.UpdatedAt = now
	if fb.Status == "" {
		fb.Status = models.FeedbackStatusNew
	}
	if fb.Severity == "" {
		fb.Severity = models.FeedbackSeverityMedium
	}

	sql := `UPSERT $rid SET
		feedback_id = $feedback_id, session_id = $session_id, client_type = $client_type,
		category = $category, severity = $severity, description = $description,
		ticker = $ticker, portfolio_name = $portfolio_name, tool_name = $tool_name,
		observed_value = $observed_value, expected_value = $expected_value,
		status = $status, resolution_notes = $resolution_notes,
		created_at = $created_at, updated_at = $updated_at`
	vars := map[string]any{
		"rid":              surrealmodels.NewRecordID("mcp_feedback", fb.ID),
		"feedback_id":      fb.ID,
		"session_id":       fb.SessionID,
		"client_type":      fb.ClientType,
		"category":         fb.Category,
		"severity":         fb.Severity,
		"description":      fb.Description,
		"ticker":           fb.Ticker,
		"portfolio_name":   fb.PortfolioName,
		"tool_name":        fb.ToolName,
		"observed_value":   fb.ObservedValue,
		"expected_value":   fb.ExpectedValue,
		"status":           fb.Status,
		"resolution_notes": fb.ResolutionNotes,
		"created_at":       fb.CreatedAt,
		"updated_at":       fb.UpdatedAt,
	}

	if _, err := surrealdb.Query[any](ctx, s.db, sql, vars); err != nil {
		return fmt.Errorf("failed to create feedback: %w", err)
	}
	return nil
}

func (s *FeedbackStore) Get(ctx context.Context, id string) (*models.Feedback, error) {
	sql := "SELECT " + feedbackSelectFields + " FROM $rid"
	vars := map[string]any{
		"rid": surrealmodels.NewRecordID("mcp_feedback", id),
	}

	results, err := surrealdb.Query[[]models.Feedback](ctx, s.db, sql, vars)
	if err != nil {
		if isNotFoundError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get feedback: %w", err)
	}

	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

func (s *FeedbackStore) List(ctx context.Context, opts interfaces.FeedbackListOptions) ([]*models.Feedback, int, error) {
	// Build WHERE clauses
	where := ""
	vars := map[string]any{}

	if opts.Status != "" {
		where += " AND status = $status"
		vars["status"] = opts.Status
	}
	if opts.Severity != "" {
		where += " AND severity = $severity"
		vars["severity"] = opts.Severity
	}
	if opts.Category != "" {
		where += " AND category = $category"
		vars["category"] = opts.Category
	}
	if opts.Ticker != "" {
		where += " AND ticker = $ticker"
		vars["ticker"] = opts.Ticker
	}
	if opts.PortfolioName != "" {
		where += " AND portfolio_name = $portfolio_name"
		vars["portfolio_name"] = opts.PortfolioName
	}
	if opts.SessionID != "" {
		where += " AND session_id = $session_id"
		vars["session_id"] = opts.SessionID
	}
	if opts.Since != nil {
		where += " AND created_at >= $since"
		vars["since"] = *opts.Since
	}
	if opts.Before != nil {
		where += " AND created_at < $before"
		vars["before"] = *opts.Before
	}

	// Strip leading " AND "
	whereClause := ""
	if where != "" {
		whereClause = " WHERE " + where[5:]
	}

	// Sort — feedback_id as tiebreaker for deterministic ordering when timestamps are equal
	orderBy := "ORDER BY created_at DESC, feedback_id DESC"
	switch opts.Sort {
	case "created_at_asc":
		orderBy = "ORDER BY created_at ASC, feedback_id ASC"
	case "severity_desc":
		orderBy = "ORDER BY (IF severity = 'high' THEN 3 ELSE IF severity = 'medium' THEN 2 ELSE 1 END) DESC, created_at DESC, feedback_id DESC"
	}

	// Count query
	countSQL := "SELECT count() AS cnt FROM mcp_feedback" + whereClause + " GROUP ALL"
	type countResult struct {
		Cnt int `json:"cnt"`
	}
	total := 0
	countResults, err := surrealdb.Query[[]countResult](ctx, s.db, countSQL, vars)
	if err == nil && countResults != nil && len(*countResults) > 0 && len((*countResults)[0].Result) > 0 {
		total = (*countResults)[0].Result[0].Cnt
	}

	// Pagination
	page := opts.Page
	if page < 1 {
		page = 1
	}
	perPage := opts.PerPage
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	offset := (page - 1) * perPage

	// Data query
	dataSQL := "SELECT " + feedbackSelectFields + " FROM mcp_feedback" + whereClause + " " + orderBy + " LIMIT $limit START $start"
	vars["limit"] = perPage
	vars["start"] = offset

	results, err := surrealdb.Query[[]models.Feedback](ctx, s.db, dataSQL, vars)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list feedback: %w", err)
	}

	items := make([]*models.Feedback, 0)
	if results != nil && len(*results) > 0 {
		for i := range (*results)[0].Result {
			items = append(items, &(*results)[0].Result[i])
		}
	}

	return items, total, nil
}

func (s *FeedbackStore) Update(ctx context.Context, id string, status, resolutionNotes string) error {
	sql := "UPDATE $rid SET status = $status, resolution_notes = $notes, updated_at = $now"
	vars := map[string]any{
		"rid":    surrealmodels.NewRecordID("mcp_feedback", id),
		"status": status,
		"notes":  resolutionNotes,
		"now":    time.Now(),
	}

	if _, err := surrealdb.Query[[]models.Feedback](ctx, s.db, sql, vars); err != nil {
		return fmt.Errorf("failed to update feedback: %w", err)
	}
	return nil
}

func (s *FeedbackStore) BulkUpdateStatus(ctx context.Context, ids []string, status, resolutionNotes string) (int, error) {
	now := time.Now()
	updated := 0
	for _, id := range ids {
		sql := "UPDATE $rid SET status = $status, resolution_notes = $notes, updated_at = $now"
		vars := map[string]any{
			"rid":    surrealmodels.NewRecordID("mcp_feedback", id),
			"status": status,
			"notes":  resolutionNotes,
			"now":    now,
		}
		if _, err := surrealdb.Query[[]models.Feedback](ctx, s.db, sql, vars); err != nil {
			s.logger.Warn().Err(err).Str("id", id).Msg("Failed to update feedback in bulk")
			continue
		}
		updated++
	}
	return updated, nil
}

func (s *FeedbackStore) Delete(ctx context.Context, id string) error {
	_, err := surrealdb.Delete[models.Feedback](ctx, s.db, surrealmodels.NewRecordID("mcp_feedback", id))
	if err != nil && !isNotFoundError(err) {
		return fmt.Errorf("failed to delete feedback: %w", err)
	}
	return nil
}

func (s *FeedbackStore) Summary(ctx context.Context) (*models.FeedbackSummary, error) {
	summary := &models.FeedbackSummary{
		ByStatus:   make(map[string]int),
		BySeverity: make(map[string]int),
		ByCategory: make(map[string]int),
	}

	// Total count
	type countResult struct {
		Cnt int `json:"cnt"`
	}
	totalSQL := "SELECT count() AS cnt FROM mcp_feedback GROUP ALL"
	totalResults, err := surrealdb.Query[[]countResult](ctx, s.db, totalSQL, nil)
	if err == nil && totalResults != nil && len(*totalResults) > 0 && len((*totalResults)[0].Result) > 0 {
		summary.Total = (*totalResults)[0].Result[0].Cnt
	}

	// By status
	type groupResult struct {
		Group string `json:"group"`
		Cnt   int    `json:"cnt"`
	}
	statusSQL := "SELECT status AS group, count() AS cnt FROM mcp_feedback GROUP BY status"
	statusResults, err := surrealdb.Query[[]groupResult](ctx, s.db, statusSQL, nil)
	if err == nil && statusResults != nil && len(*statusResults) > 0 {
		for _, r := range (*statusResults)[0].Result {
			summary.ByStatus[r.Group] = r.Cnt
		}
	}

	// By severity
	sevSQL := "SELECT severity AS group, count() AS cnt FROM mcp_feedback GROUP BY severity"
	sevResults, err := surrealdb.Query[[]groupResult](ctx, s.db, sevSQL, nil)
	if err == nil && sevResults != nil && len(*sevResults) > 0 {
		for _, r := range (*sevResults)[0].Result {
			summary.BySeverity[r.Group] = r.Cnt
		}
	}

	// By category
	catSQL := "SELECT category AS group, count() AS cnt FROM mcp_feedback GROUP BY category"
	catResults, err := surrealdb.Query[[]groupResult](ctx, s.db, catSQL, nil)
	if err == nil && catResults != nil && len(*catResults) > 0 {
		for _, r := range (*catResults)[0].Result {
			summary.ByCategory[r.Group] = r.Cnt
		}
	}

	// Oldest unresolved
	type timeResult struct {
		CreatedAt time.Time `json:"created_at"`
	}
	oldestSQL := "SELECT created_at FROM mcp_feedback WHERE status IN [$new, $ack] ORDER BY created_at ASC LIMIT 1"
	oldestVars := map[string]any{
		"new": models.FeedbackStatusNew,
		"ack": models.FeedbackStatusAcknowledged,
	}
	oldestResults, err := surrealdb.Query[[]timeResult](ctx, s.db, oldestSQL, oldestVars)
	if err == nil && oldestResults != nil && len(*oldestResults) > 0 && len((*oldestResults)[0].Result) > 0 {
		t := (*oldestResults)[0].Result[0].CreatedAt
		summary.OldestUnresolved = &t
	}

	return summary, nil
}

// Compile-time check
var _ interfaces.FeedbackStore = (*FeedbackStore)(nil)
