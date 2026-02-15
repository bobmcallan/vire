package badger

import (
	"context"
	"fmt"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/timshannon/badgerhold/v4"
)

type userStorage struct {
	store  *Store
	logger *common.Logger
}

// NewUserStorage creates a new UserStorage backed by BadgerHold.
func NewUserStorage(store *Store, logger *common.Logger) *userStorage {
	return &userStorage{store: store, logger: logger}
}

func (s *userStorage) GetUser(_ context.Context, username string) (*models.User, error) {
	var user models.User
	err := s.store.db.Get(username, &user)
	if err != nil {
		if err == badgerhold.ErrNotFound {
			return nil, fmt.Errorf("user '%s' not found", username)
		}
		return nil, fmt.Errorf("failed to get user '%s': %w", username, err)
	}
	return &user, nil
}

func (s *userStorage) SaveUser(_ context.Context, user *models.User) error {
	if err := s.store.db.Upsert(user.Username, user); err != nil {
		return fmt.Errorf("failed to save user: %w", err)
	}
	s.logger.Debug().Str("username", user.Username).Msg("User saved")
	return nil
}

func (s *userStorage) DeleteUser(_ context.Context, username string) error {
	err := s.store.db.Delete(username, models.User{})
	if err != nil && err != badgerhold.ErrNotFound {
		return fmt.Errorf("failed to delete user '%s': %w", username, err)
	}
	s.logger.Debug().Str("username", username).Msg("User deleted")
	return nil
}

func (s *userStorage) ListUsers(_ context.Context) ([]string, error) {
	var users []models.User
	if err := s.store.db.Find(&users, nil); err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	names := make([]string, len(users))
	for i, u := range users {
		names[i] = u.Username
	}
	return names, nil
}
