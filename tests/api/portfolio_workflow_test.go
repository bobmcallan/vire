package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/tests/common"
)

func TestPortfolioWorkflow(t *testing.T) {
	// Load tests/docker/.env (shell env vars take precedence)
	envPath := filepath.Join(common.FindProjectRoot(), "tests", "docker", ".env")
	if err := common.LoadEnvFile(envPath); err != nil {
		t.Logf("Warning: could not load %s: %v", envPath, err)
	}

	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	navexaKey := os.Getenv("NAVEXA_API_KEY")
	if navexaKey == "" {
		t.Skip("NAVEXA_API_KEY not set (set in env or tests/docker/.env)")
	}
	defaultPortfolio := os.Getenv("DEFAULT_PORTFOLIO")
	if defaultPortfolio == "" {
		t.Skip("DEFAULT_PORTFOLIO not set (set in env or tests/docker/.env)")
	}

	userHeaders := map[string]string{"X-Vire-User-ID": "dev_user"}

	// Step 1: Import users from tests/fixtures/users.json
	t.Run("import_users", func(t *testing.T) {
		usersPath := filepath.Join(common.FindProjectRoot(), "tests", "fixtures", "users.json")
		data, err := os.ReadFile(usersPath)
		require.NoError(t, err)

		var usersFile struct {
			Users []json.RawMessage `json:"users"`
		}
		require.NoError(t, json.Unmarshal(data, &usersFile))
		require.NotEmpty(t, usersFile.Users, "users.json should contain at least one user")

		for i, userRaw := range usersFile.Users {
			resp, err := env.HTTPPost("/api/users/upsert", json.RawMessage(userRaw))
			require.NoError(t, err)

			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			require.NoError(t, err)

			assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated,
				"user %d: expected 200 or 201, got %d: %s", i, resp.StatusCode, string(body))

			t.Logf("user %d: status=%d", i, resp.StatusCode)
		}
	})

	// Step 2: Set Navexa key on dev_user
	t.Run("set_navexa_key", func(t *testing.T) {
		resp, err := env.HTTPPut("/api/users/dev_user", map[string]string{
			"navexa_key": navexaKey,
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode, "set navexa key: %s", string(body))

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))

		// Check nested data.navexa_key_set if present
		if data, ok := result["data"].(map[string]interface{}); ok {
			assert.Equal(t, true, data["navexa_key_set"], "navexa_key_set should be true")
		}

		env.SaveResult("set_navexa_key.json", body)
		t.Logf("set_navexa_key response: %s", string(body))
	})

	// Step 3: Get Portfolios — assert > 1 returned
	t.Run("get_portfolios", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode, "get portfolios: %s", string(body))

		var result struct {
			Portfolios []interface{} `json:"portfolios"`
		}
		require.NoError(t, json.Unmarshal(body, &result))
		assert.Greater(t, len(result.Portfolios), 1, "expected more than 1 portfolio")

		env.SaveResult("get_portfolios.json", body)
		t.Logf("get_portfolios: found %d portfolios", len(result.Portfolios))
	})

	// Step 4: Set default portfolio
	t.Run("set_default_portfolio", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPut, "/api/portfolios/default",
			map[string]string{"name": defaultPortfolio}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode, "set default portfolio: %s", string(body))

		env.SaveResult("set_default_portfolio.json", body)
		t.Logf("set_default_portfolio response: %s", string(body))
	})

	// Step 5: Get portfolio — assert response < 120 seconds
	t.Run("get_portfolio", func(t *testing.T) {
		start := time.Now()

		resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+defaultPortfolio, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		elapsed := time.Since(start)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode, "get portfolio: %s", string(body))
		assert.Less(t, elapsed, 120*time.Second, "portfolio fetch took too long: %s", elapsed)

		env.SaveResult("get_portfolio.json", body)
		t.Logf("get_portfolio: status=%d elapsed=%s body_len=%d", resp.StatusCode, elapsed, len(body))
	})
}
