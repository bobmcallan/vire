package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"golang.org/x/crypto/bcrypt"
)

type importUsersFile struct {
	Users []importUser `json:"users"`
}

type importUser struct {
	Username        string   `json:"username"`
	Email           string   `json:"email"`
	Password        string   `json:"password"`
	Role            string   `json:"role"`
	DisplayCurrency string   `json:"display_currency"`
	Portfolios      []string `json:"portfolios"`
}

// ImportUsersFromFile reads a users JSON file and imports users into storage.
// Existing users (by username) are skipped. Passwords are bcrypt-hashed.
// When devMode is false, passwords from the file are ignored — a random password
// is generated for each user and logged so the operator can retrieve it.
// Returns (imported count, skipped count, error).
func ImportUsersFromFile(ctx context.Context, store interfaces.InternalStore, logger *common.Logger, filePath string, devMode bool) (int, int, error) {
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
		if _, err := store.GetUser(ctx, u.Username); err == nil {
			skipped++
			continue
		}

		// Resolve password: use file password in dev, random in prod
		password := u.Password
		if !devMode {
			password, err = generateRandomPassword(16)
			if err != nil {
				logger.Warn().Err(err).Str("username", u.Username).Msg("Failed to generate random password during import")
				skipped++
				continue
			}
		}

		// Hash password
		passwordBytes := []byte(password)
		if len(passwordBytes) > 72 {
			passwordBytes = passwordBytes[:72]
		}
		hash, err := bcrypt.GenerateFromPassword(passwordBytes, 10)
		if err != nil {
			logger.Warn().Err(err).Str("username", u.Username).Msg("Failed to hash password during import")
			skipped++
			continue
		}
		user := &models.InternalUser{
			UserID:       u.Username,
			Email:        u.Email,
			PasswordHash: string(hash),
			Role:         u.Role,
			CreatedAt:    time.Now(),
		}
		if err := store.SaveUser(ctx, user); err != nil {
			logger.Warn().Err(err).Str("username", u.Username).Msg("Failed to save user during import")
			skipped++
			continue
		}
		// Save preferences as UserKV entries
		if u.DisplayCurrency != "" {
			store.SetUserKV(ctx, u.Username, "display_currency", u.DisplayCurrency)
		}
		if len(u.Portfolios) > 0 {
			store.SetUserKV(ctx, u.Username, "portfolios", strings.Join(u.Portfolios, ","))
		}

		logEvent := logger.Info().Str("username", u.Username).Str("role", u.Role)
		if !devMode {
			logEvent = logEvent.Str("password", password)
		}
		logEvent.Msg("User imported")
		imported++
	}
	return imported, skipped, nil
}

func generateRandomPassword(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
