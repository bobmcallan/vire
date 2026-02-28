package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"golang.org/x/crypto/bcrypt"
)

// ensureBreakglassAdmin creates the break-glass admin user if it does not already exist.
// Returns the cleartext password if a new user was created, or "" if the user already exists.
func ensureBreakglassAdmin(ctx context.Context, store interfaces.InternalStore, logger *common.Logger) string {
	// Check if user already exists (idempotent)
	if _, err := store.GetUser(ctx, "breakglass-admin"); err == nil {
		logger.Info().Msg("Break-glass admin already exists")
		return ""
	}

	// Generate 24-char cryptographically random password
	buf := make([]byte, 18) // 18 bytes -> 24 chars in base64
	if _, err := rand.Read(buf); err != nil {
		logger.Error().Err(err).Msg("Failed to generate random password for break-glass admin")
		return ""
	}
	password := base64.RawURLEncoding.EncodeToString(buf)

	// bcrypt hash (cost 10, truncate to 72 bytes like existing code)
	passwordBytes := []byte(password)
	if len(passwordBytes) > 72 {
		passwordBytes = passwordBytes[:72]
	}
	hash, err := bcrypt.GenerateFromPassword(passwordBytes, 10)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to hash break-glass admin password")
		return ""
	}

	user := &models.InternalUser{
		UserID:       "breakglass-admin",
		Email:        "admin@vire.local",
		Name:         "Break-Glass Admin",
		PasswordHash: string(hash),
		Provider:     "system",
		Role:         models.RoleAdmin,
		CreatedAt:    time.Now(),
	}

	if err := store.SaveUser(ctx, user); err != nil {
		logger.Error().Err(err).Msg("Failed to save break-glass admin user")
		return ""
	}

	logger.Warn().
		Str("email", "admin@vire.local").
		Str("password", password).
		Msg("Break-glass admin created")

	return password
}
