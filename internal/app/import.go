package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"golang.org/x/crypto/bcrypt"
)

type importUsersFile struct {
	Users []importUser `json:"users"`
}

type importUser struct {
	Username         string   `json:"username"`
	Email            string   `json:"email"`
	Password         string   `json:"password"`
	Role             string   `json:"role"`
	NavexaKey        string   `json:"navexa_key"`
	DisplayCurrency  string   `json:"display_currency"`
	DefaultPortfolio string   `json:"default_portfolio"`
	Portfolios       []string `json:"portfolios"`
}

// ImportUsersFromFile reads a users JSON file and imports users into storage.
// Existing users (by username) are skipped. Passwords are bcrypt-hashed.
// Returns (imported count, skipped count, error).
func ImportUsersFromFile(ctx context.Context, userStore interfaces.UserStorage, logger *common.Logger, filePath string) (int, int, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read users file %s: %w", filePath, err)
	}

	var file importUsersFile
	if err := json.Unmarshal(data, &file); err != nil {
		return 0, 0, fmt.Errorf("failed to parse users file %s: %w", filePath, err)
	}

	imported, skipped := 0, 0
	for _, u := range file.Users {
		if u.Username == "" {
			skipped++
			continue
		}
		// Skip if exists
		if _, err := userStore.GetUser(ctx, u.Username); err == nil {
			skipped++
			continue
		}
		// Hash password
		passwordBytes := []byte(u.Password)
		if len(passwordBytes) > 72 {
			passwordBytes = passwordBytes[:72]
		}
		hash, err := bcrypt.GenerateFromPassword(passwordBytes, 10)
		if err != nil {
			logger.Warn().Err(err).Str("username", u.Username).Msg("Failed to hash password during import")
			skipped++
			continue
		}
		user := &models.User{
			Username:         u.Username,
			Email:            u.Email,
			PasswordHash:     string(hash),
			Role:             u.Role,
			NavexaKey:        u.NavexaKey,
			DisplayCurrency:  u.DisplayCurrency,
			DefaultPortfolio: u.DefaultPortfolio,
			Portfolios:       u.Portfolios,
		}
		if err := userStore.SaveUser(ctx, user); err != nil {
			logger.Warn().Err(err).Str("username", u.Username).Msg("Failed to save user during import")
			skipped++
			continue
		}
		logger.Info().Str("username", u.Username).Str("role", u.Role).Msg("User imported")
		imported++
	}
	return imported, skipped, nil
}
