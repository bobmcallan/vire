package surrealdb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/google/uuid"
	"github.com/surrealdb/surrealdb.go"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// changelogSelectFields lists the fields to select from changelog, aliasing changelog_id to id for struct mapping.
const changelogSelectFields = `changelog_id as id, service, service_version, service_build, content,
	created_by_id, created_by_name, updated_by_id, updated_by_name,
	created_at, updated_at`

// ChangelogStore implements interfaces.ChangelogStore using SurrealDB.
type ChangelogStore struct {
	db     *surrealdb.DB
	logger *common.Logger
}

// NewChangelogStore creates a new ChangelogStore.
func NewChangelogStore(db *surrealdb.DB, logger *common.Logger) *ChangelogStore {
	return &ChangelogStore{db: db, logger: logger}
}

func (s *ChangelogStore) Create(ctx context.Context, entry *models.ChangelogEntry) error {
	if entry.ID == "" {
		entry.ID = fmt.Sprintf("cl_%s", uuid.New().String()[:8])
	}
	now := time.Now()
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	entry.UpdatedAt = now

	sql := `UPSERT $rid SET
		changelog_id = $changelog_id, service = $service, service_version = $service_version,
		service_build = $service_build, content = $content,
		created_by_id = $created_by_id, created_by_name = $created_by_name,
		updated_by_id = $updated_by_id, updated_by_name = $updated_by_name,
		created_at = $created_at, updated_at = $updated_at`
	vars := map[string]any{
		"rid":             surrealmodels.NewRecordID("changelog", entry.ID),
		"changelog_id":    entry.ID,
		"service":         entry.Service,
		"service_version": entry.ServiceVersion,
		"service_build":   entry.ServiceBuild,
		"content":         entry.Content,
		"created_by_id":   entry.CreatedByID,
		"created_by_name": entry.CreatedByName,
		"updated_by_id":   entry.UpdatedByID,
		"updated_by_name": entry.UpdatedByName,
		"created_at":      entry.CreatedAt,
		"updated_at":      entry.UpdatedAt,
	}

	if _, err := surrealdb.Query[any](ctx, s.db, sql, vars); err != nil {
		return fmt.Errorf("failed to create changelog entry: %w", err)
	}
	return nil
}

func (s *ChangelogStore) Get(ctx context.Context, id string) (*models.ChangelogEntry, error) {
	sql := "SELECT " + changelogSelectFields + " FROM $rid"
	vars := map[string]any{
		"rid": surrealmodels.NewRecordID("changelog", id),
	}

	results, err := surrealdb.Query[[]models.ChangelogEntry](ctx, s.db, sql, vars)
	if err != nil {
		if isNotFoundError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get changelog entry: %w", err)
	}

	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

func (s *ChangelogStore) List(ctx context.Context, opts interfaces.ChangelogListOptions) ([]*models.ChangelogEntry, int, error) {
	// Build WHERE clauses
	where := ""
	vars := map[string]any{}

	if opts.Service != "" {
		where += " AND service = $service"
		vars["service"] = opts.Service
	}

	// Strip leading " AND "
	whereClause := ""
	if where != "" {
		whereClause = " WHERE " + where[5:]
	}

	orderBy := "ORDER BY created_at DESC, changelog_id DESC"

	// Count query
	countSQL := "SELECT count() AS cnt FROM changelog" + whereClause + " GROUP ALL"
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
	dataSQL := "SELECT " + changelogSelectFields + " FROM changelog" + whereClause + " " + orderBy + " LIMIT $limit START $start"
	vars["limit"] = perPage
	vars["start"] = offset

	results, err := surrealdb.Query[[]models.ChangelogEntry](ctx, s.db, dataSQL, vars)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list changelog entries: %w", err)
	}

	items := make([]*models.ChangelogEntry, 0)
	if results != nil && len(*results) > 0 {
		for i := range (*results)[0].Result {
			items = append(items, &(*results)[0].Result[i])
		}
	}

	return items, total, nil
}

func (s *ChangelogStore) Update(ctx context.Context, entry *models.ChangelogEntry) error {
	sets := []string{"updated_at = $now"}
	vars := map[string]any{
		"rid": surrealmodels.NewRecordID("changelog", entry.ID),
		"now": time.Now(),
	}
	if entry.Service != "" {
		sets = append(sets, "service = $service")
		vars["service"] = entry.Service
	}
	if entry.ServiceVersion != "" {
		sets = append(sets, "service_version = $service_version")
		vars["service_version"] = entry.ServiceVersion
	}
	if entry.ServiceBuild != "" {
		sets = append(sets, "service_build = $service_build")
		vars["service_build"] = entry.ServiceBuild
	}
	if entry.Content != "" {
		sets = append(sets, "content = $content")
		vars["content"] = entry.Content
	}
	if entry.UpdatedByID != "" {
		sets = append(sets, "updated_by_id = $updated_by_id")
		vars["updated_by_id"] = entry.UpdatedByID
	}
	if entry.UpdatedByName != "" {
		sets = append(sets, "updated_by_name = $updated_by_name")
		vars["updated_by_name"] = entry.UpdatedByName
	}

	sql := "UPDATE $rid SET " + strings.Join(sets, ", ")
	if _, err := surrealdb.Query[[]models.ChangelogEntry](ctx, s.db, sql, vars); err != nil {
		return fmt.Errorf("failed to update changelog entry: %w", err)
	}
	return nil
}

func (s *ChangelogStore) Delete(ctx context.Context, id string) error {
	_, err := surrealdb.Delete[models.ChangelogEntry](ctx, s.db, surrealmodels.NewRecordID("changelog", id))
	if err != nil && !isNotFoundError(err) {
		return fmt.Errorf("failed to delete changelog entry: %w", err)
	}
	return nil
}

// Compile-time check
var _ interfaces.ChangelogStore = (*ChangelogStore)(nil)
