package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRole(t *testing.T) {
	tests := []struct {
		name    string
		role    string
		wantErr bool
	}{
		{"admin_valid", RoleAdmin, false},
		{"user_valid", RoleUser, false},
		{"empty_invalid", "", true},
		{"superadmin_invalid", "superadmin", true},
		{"capitalized_Admin", "Admin", true},
		{"uppercase_USER", "USER", true},
		{"root_invalid", "root", true},
		{"moderator_invalid", "moderator", true},
		{"leading_space", " admin", true},
		{"trailing_space", "admin ", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRole(tt.role)
			if tt.wantErr {
				assert.Error(t, err, "ValidateRole(%q) should return error", tt.role)
			} else {
				require.NoError(t, err, "ValidateRole(%q) should not return error", tt.role)
			}
		})
	}
}

func TestValidateRole_ErrorMessage(t *testing.T) {
	err := ValidateRole("badrole")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "badrole", "error should mention the invalid role")
	assert.Contains(t, err.Error(), RoleAdmin, "error should mention valid roles")
	assert.Contains(t, err.Error(), RoleUser, "error should mention valid roles")
}

func TestRoleConstants(t *testing.T) {
	assert.Equal(t, "admin", RoleAdmin)
	assert.Equal(t, "user", RoleUser)
}
