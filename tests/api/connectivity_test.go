package api

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/internal/clients/eodhd"
	"github.com/bobmcallan/vire/internal/clients/gemini"
	"github.com/bobmcallan/vire/internal/clients/navexa"
	"github.com/bobmcallan/vire/internal/common"
	tcommon "github.com/bobmcallan/vire/tests/common"
)

// TestNavexaConnectivity verifies the Navexa API key and connection
func TestNavexaConnectivity(t *testing.T) {
	tcommon.LoadEnvFile(filepath.Join(tcommon.FindProjectRoot(), "tests", "docker", ".env"))
	apiKey := os.Getenv("NAVEXA_API_KEY")
	if apiKey == "" {
		t.Skip("NAVEXA_API_KEY not set")
	}

	logger := common.NewLogger("debug")
	client := navexa.NewClient(apiKey, navexa.WithLogger(logger))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	portfolios, err := client.GetPortfolios(ctx)
	require.NoError(t, err, "Navexa GetPortfolios failed — check API key")
	require.NotEmpty(t, portfolios, "Navexa returned no portfolios")

	t.Logf("Navexa OK: %d portfolios", len(portfolios))
	for _, p := range portfolios {
		t.Logf("  - %s (ID: %s)", p.Name, p.ID)
	}

	// Test holdings for first portfolio
	holdings, err := client.GetHoldings(ctx, portfolios[0].ID)
	require.NoError(t, err, "Navexa GetHoldings failed")
	t.Logf("  Holdings: %d", len(holdings))
}

// TestEODHDConnectivity verifies the EODHD API key and connection
func TestEODHDConnectivity(t *testing.T) {
	tcommon.LoadEnvFile(filepath.Join(tcommon.FindProjectRoot(), "tests", "docker", ".env"))
	apiKey := os.Getenv("EODHD_API_KEY")
	if apiKey == "" {
		t.Skip("EODHD_API_KEY not set")
	}

	logger := common.NewLogger("debug")
	client := eodhd.NewClient(apiKey, eodhd.WithLogger(logger))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	eodResp, err := client.GetEOD(ctx, "BHP.AU")
	require.NoError(t, err, "EODHD GetEOD failed — check API key")
	require.NotNil(t, eodResp, "EODHD returned nil response")
	require.NotEmpty(t, eodResp.Data, "EODHD returned no price data")

	last := eodResp.Data[len(eodResp.Data)-1]
	t.Logf("EODHD OK: BHP.AU — %d bars, last close: %.2f", len(eodResp.Data), last.Close)
}

// TestGeminiConnectivity verifies the Gemini API key and connection
func TestGeminiConnectivity(t *testing.T) {
	tcommon.LoadEnvFile(filepath.Join(tcommon.FindProjectRoot(), "tests", "docker", ".env"))
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("GEMINI_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger := common.NewLogger("debug")
	client, err := gemini.NewClient(ctx, apiKey, gemini.WithLogger(logger))
	require.NoError(t, err, "Gemini client creation failed — check API key")

	result, err := client.GenerateContent(ctx, "Reply with exactly: CONNECTIVITY_OK")
	require.NoError(t, err, "Gemini GenerateContent failed")
	assert.Contains(t, result, "CONNECTIVITY_OK")

	t.Logf("Gemini OK: %s", result)
}
