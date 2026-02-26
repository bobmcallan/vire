package surrealdb

import (
	"context"
	"fmt"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/surrealdb/surrealdb.go"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// oauthClientRow is the DB-level representation of an OAuth client.
type oauthClientRow struct {
	ClientID         string    `json:"client_id"`
	ClientSecretHash string    `json:"client_secret_hash"`
	ClientName       string    `json:"client_name"`
	RedirectURIs     []string  `json:"redirect_uris"`
	CreatedAt        time.Time `json:"created_at"`
}

// oauthCodeRow is the DB-level representation of an OAuth authorization code.
type oauthCodeRow struct {
	Code                string    `json:"code"`
	ClientID            string    `json:"client_id"`
	UserID              string    `json:"user_id"`
	RedirectURI         string    `json:"redirect_uri"`
	CodeChallenge       string    `json:"code_challenge"`
	CodeChallengeMethod string    `json:"code_challenge_method"`
	Scope               string    `json:"scope"`
	ExpiresAt           time.Time `json:"expires_at"`
	Used                bool      `json:"used"`
	CreatedAt           time.Time `json:"created_at"`
}

// oauthRefreshRow is the DB-level representation of an OAuth refresh token.
type oauthRefreshRow struct {
	TokenHash  string    `json:"token_hash"`
	ClientID   string    `json:"client_id"`
	UserID     string    `json:"user_id"`
	Scope      string    `json:"scope"`
	ExpiresAt  time.Time `json:"expires_at"`
	Revoked    bool      `json:"revoked"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at"`
}

// OAuthStore implements interfaces.OAuthStore using SurrealDB.
type OAuthStore struct {
	db     *surrealdb.DB
	logger *common.Logger
}

// NewOAuthStore creates a new OAuthStore.
func NewOAuthStore(db *surrealdb.DB, logger *common.Logger) *OAuthStore {
	return &OAuthStore{db: db, logger: logger}
}

// --- Clients ---

func (s *OAuthStore) SaveClient(ctx context.Context, client *models.OAuthClient) error {
	sql := `UPSERT $rid SET
		client_id = $client_id, client_secret_hash = $client_secret_hash,
		client_name = $client_name, redirect_uris = $redirect_uris,
		created_at = $created_at`
	vars := map[string]any{
		"rid":                surrealmodels.NewRecordID("oauth_client", client.ClientID),
		"client_id":          client.ClientID,
		"client_secret_hash": client.ClientSecretHash,
		"client_name":        client.ClientName,
		"redirect_uris":      client.RedirectURIs,
		"created_at":         client.CreatedAt,
	}
	if _, err := surrealdb.Query[any](ctx, s.db, sql, vars); err != nil {
		return fmt.Errorf("failed to save oauth client: %w", err)
	}
	return nil
}

func (s *OAuthStore) GetClient(ctx context.Context, clientID string) (*models.OAuthClient, error) {
	sql := "SELECT client_id, client_secret_hash, client_name, redirect_uris, created_at FROM $rid"
	vars := map[string]any{
		"rid": surrealmodels.NewRecordID("oauth_client", clientID),
	}
	results, err := surrealdb.Query[[]oauthClientRow](ctx, s.db, sql, vars)
	if err != nil {
		if isNotFoundError(err) {
			return nil, fmt.Errorf("oauth client not found: %s", clientID)
		}
		return nil, fmt.Errorf("failed to get oauth client: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, fmt.Errorf("oauth client not found: %s", clientID)
	}
	row := (*results)[0].Result[0]
	return &models.OAuthClient{
		ClientID:         row.ClientID,
		ClientSecretHash: row.ClientSecretHash,
		ClientName:       row.ClientName,
		RedirectURIs:     row.RedirectURIs,
		CreatedAt:        row.CreatedAt,
	}, nil
}

func (s *OAuthStore) DeleteClient(ctx context.Context, clientID string) error {
	rid := surrealmodels.NewRecordID("oauth_client", clientID)
	_, err := surrealdb.Delete[oauthClientRow](ctx, s.db, rid)
	if err != nil && !isNotFoundError(err) {
		return fmt.Errorf("failed to delete oauth client: %w", err)
	}
	return nil
}

// --- Authorization codes ---

func (s *OAuthStore) SaveCode(ctx context.Context, code *models.OAuthCode) error {
	sql := `UPSERT $rid SET
		code = $code, client_id = $client_id, user_id = $user_id,
		redirect_uri = $redirect_uri, code_challenge = $code_challenge,
		code_challenge_method = $code_challenge_method, scope = $scope,
		expires_at = $expires_at, used = $used, created_at = $created_at`
	vars := map[string]any{
		"rid":                   surrealmodels.NewRecordID("oauth_code", code.Code),
		"code":                  code.Code,
		"client_id":             code.ClientID,
		"user_id":               code.UserID,
		"redirect_uri":          code.RedirectURI,
		"code_challenge":        code.CodeChallenge,
		"code_challenge_method": code.CodeChallengeMethod,
		"scope":                 code.Scope,
		"expires_at":            code.ExpiresAt,
		"used":                  code.Used,
		"created_at":            code.CreatedAt,
	}
	if _, err := surrealdb.Query[any](ctx, s.db, sql, vars); err != nil {
		return fmt.Errorf("failed to save oauth code: %w", err)
	}
	return nil
}

func (s *OAuthStore) GetCode(ctx context.Context, code string) (*models.OAuthCode, error) {
	sql := "SELECT code, client_id, user_id, redirect_uri, code_challenge, code_challenge_method, scope, expires_at, used, created_at FROM $rid"
	vars := map[string]any{
		"rid": surrealmodels.NewRecordID("oauth_code", code),
	}
	results, err := surrealdb.Query[[]oauthCodeRow](ctx, s.db, sql, vars)
	if err != nil {
		if isNotFoundError(err) {
			return nil, fmt.Errorf("oauth code not found")
		}
		return nil, fmt.Errorf("failed to get oauth code: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, fmt.Errorf("oauth code not found")
	}
	row := (*results)[0].Result[0]
	return &models.OAuthCode{
		Code:                row.Code,
		ClientID:            row.ClientID,
		UserID:              row.UserID,
		RedirectURI:         row.RedirectURI,
		CodeChallenge:       row.CodeChallenge,
		CodeChallengeMethod: row.CodeChallengeMethod,
		Scope:               row.Scope,
		ExpiresAt:           row.ExpiresAt,
		Used:                row.Used,
		CreatedAt:           row.CreatedAt,
	}, nil
}

func (s *OAuthStore) MarkCodeUsed(ctx context.Context, code string) error {
	sql := "UPDATE $rid SET used = true"
	vars := map[string]any{
		"rid": surrealmodels.NewRecordID("oauth_code", code),
	}
	if _, err := surrealdb.Query[any](ctx, s.db, sql, vars); err != nil {
		return fmt.Errorf("failed to mark oauth code used: %w", err)
	}
	return nil
}

func (s *OAuthStore) PurgeExpiredCodes(ctx context.Context) (int, error) {
	sql := "DELETE FROM oauth_code WHERE expires_at < $now"
	vars := map[string]any{"now": time.Now()}
	if _, err := surrealdb.Query[any](ctx, s.db, sql, vars); err != nil {
		return 0, fmt.Errorf("failed to purge expired codes: %w", err)
	}
	// SurrealDB DELETE doesn't return count easily; return 0 as best effort
	return 0, nil
}

// --- Refresh tokens ---

func (s *OAuthStore) SaveRefreshToken(ctx context.Context, token *models.OAuthRefreshToken) error {
	sql := `UPSERT $rid SET
		token_hash = $token_hash, client_id = $client_id, user_id = $user_id,
		scope = $scope, expires_at = $expires_at, revoked = $revoked,
		created_at = $created_at, last_used_at = $last_used_at`
	vars := map[string]any{
		"rid":          surrealmodels.NewRecordID("oauth_refresh_token", token.TokenHash),
		"token_hash":   token.TokenHash,
		"client_id":    token.ClientID,
		"user_id":      token.UserID,
		"scope":        token.Scope,
		"expires_at":   token.ExpiresAt,
		"revoked":      token.Revoked,
		"created_at":   token.CreatedAt,
		"last_used_at": token.LastUsedAt,
	}
	if _, err := surrealdb.Query[any](ctx, s.db, sql, vars); err != nil {
		return fmt.Errorf("failed to save refresh token: %w", err)
	}
	return nil
}

func (s *OAuthStore) GetRefreshToken(ctx context.Context, tokenHash string) (*models.OAuthRefreshToken, error) {
	sql := "SELECT token_hash, client_id, user_id, scope, expires_at, revoked, created_at, last_used_at FROM $rid"
	vars := map[string]any{
		"rid": surrealmodels.NewRecordID("oauth_refresh_token", tokenHash),
	}
	results, err := surrealdb.Query[[]oauthRefreshRow](ctx, s.db, sql, vars)
	if err != nil {
		if isNotFoundError(err) {
			return nil, fmt.Errorf("refresh token not found")
		}
		return nil, fmt.Errorf("failed to get refresh token: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, fmt.Errorf("refresh token not found")
	}
	row := (*results)[0].Result[0]
	return &models.OAuthRefreshToken{
		TokenHash:  row.TokenHash,
		ClientID:   row.ClientID,
		UserID:     row.UserID,
		Scope:      row.Scope,
		ExpiresAt:  row.ExpiresAt,
		Revoked:    row.Revoked,
		CreatedAt:  row.CreatedAt,
		LastUsedAt: row.LastUsedAt,
	}, nil
}

func (s *OAuthStore) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	sql := "UPDATE $rid SET revoked = true"
	vars := map[string]any{
		"rid": surrealmodels.NewRecordID("oauth_refresh_token", tokenHash),
	}
	if _, err := surrealdb.Query[any](ctx, s.db, sql, vars); err != nil {
		return fmt.Errorf("failed to revoke refresh token: %w", err)
	}
	return nil
}

func (s *OAuthStore) RevokeRefreshTokensByClient(ctx context.Context, userID, clientID string) error {
	sql := "UPDATE oauth_refresh_token SET revoked = true WHERE user_id = $user_id AND client_id = $client_id"
	vars := map[string]any{
		"user_id":   userID,
		"client_id": clientID,
	}
	if _, err := surrealdb.Query[any](ctx, s.db, sql, vars); err != nil {
		return fmt.Errorf("failed to revoke refresh tokens by client: %w", err)
	}
	return nil
}

func (s *OAuthStore) PurgeExpiredTokens(ctx context.Context) (int, error) {
	sql := "DELETE FROM oauth_refresh_token WHERE expires_at < $now"
	vars := map[string]any{"now": time.Now()}
	if _, err := surrealdb.Query[any](ctx, s.db, sql, vars); err != nil {
		return 0, fmt.Errorf("failed to purge expired tokens: %w", err)
	}
	return 0, nil
}

func (s *OAuthStore) UpdateRefreshTokenLastUsed(ctx context.Context, tokenHash string, lastUsedAt time.Time) error {
	sql := "UPDATE $rid SET last_used_at = $last_used_at"
	vars := map[string]any{
		"rid":          surrealmodels.NewRecordID("oauth_refresh_token", tokenHash),
		"last_used_at": lastUsedAt,
	}
	if _, err := surrealdb.Query[any](ctx, s.db, sql, vars); err != nil {
		return fmt.Errorf("failed to update refresh token last_used_at: %w", err)
	}
	return nil
}

// Compile-time check
var _ interfaces.OAuthStore = (*OAuthStore)(nil)
