package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/tests/common"
)

// TestRefreshStockData_EnqueuesLivePriceJob verifies that POST /api/market/refresh
// enqueues live price collection jobs for the exchanges of the requested tickers.
// Uses blank config (no real API key) — the test only checks that jobs are enqueued,
// not that live prices are actually fetched.
func TestRefreshStockData_EnqueuesLivePriceJob(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ConfigFile: "vire-service-blank.toml",
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	headers := map[string]string{"X-Vire-User-ID": "live-price-test-user"}

	// Call POST /api/market/refresh with AU tickers
	reqBody := strings.NewReader(`{"tickers":["BHP.AU","CBA.AU"]}`)
	resp, err := env.HTTPRequest(http.MethodPost, "/api/market/refresh", reqBody, headers)
	require.NoError(t, err, "refresh request should not fail")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	guard.SaveResult("01_refresh_response.json", string(body))

	t.Run("returns_success", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, resp.StatusCode,
			"refresh should return 200: %s", string(body))
	})

	t.Run("response_has_batch_id", func(t *testing.T) {
		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))
		assert.Contains(t, result, "batch_id", "response should contain batch_id")
		assert.Contains(t, result, "jobs_enqueued", "response should contain jobs_enqueued")
	})

	// Check the job queue for collect_live_prices jobs
	resp2, err := env.HTTPRequest(http.MethodGet, "/api/market/refresh/status", nil, headers)
	require.NoError(t, err)
	defer resp2.Body.Close()

	statusBody, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)

	guard.SaveResult("02_queue_status.json", string(statusBody))

	t.Run("queue_has_pending_jobs", func(t *testing.T) {
		var status map[string]interface{}
		require.NoError(t, json.Unmarshal(statusBody, &status))

		// The queue should have at least some pending/running jobs (including live price)
		totalPending, _ := status["total_pending"].(float64)
		totalRunning, _ := status["total_running"].(float64)
		assert.Greater(t, totalPending+totalRunning, float64(0),
			"queue should have jobs after refresh: %s", string(statusBody))
	})

	// Also check admin job list for live price job type
	resp3, err := env.HTTPRequest(http.MethodGet, "/api/admin/jobs?status=pending", nil, headers)
	if err == nil && resp3 != nil {
		defer resp3.Body.Close()
		jobsBody, _ := io.ReadAll(resp3.Body)
		guard.SaveResult("03_admin_jobs.json", string(jobsBody))

		t.Run("live_price_job_enqueued", func(t *testing.T) {
			// The response should contain a collect_live_prices job for AU exchange
			assert.Contains(t, string(jobsBody), "collect_live_prices",
				"admin jobs should include collect_live_prices job")
		})
	}
}
