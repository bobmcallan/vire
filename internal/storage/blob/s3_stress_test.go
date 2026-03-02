package blob

import (
	"testing"
)

// Devils-advocate stress tests for S3 blob store key construction.
// These are unit-level tests that don't require an S3 endpoint.
// They verify the objectKey function handles adversarial inputs safely.

// ============================================================================
// S3S-1. Object key injection — special characters in category/key
// ============================================================================

func TestS3Stress_ObjectKey_SpecialChars(t *testing.T) {
	store := &S3Store{prefix: "vire"}

	cases := []struct {
		category string
		key      string
		expected string
		desc     string
	}{
		{"filing_pdf", "BHP/doc.pdf", "vire/filing_pdf/BHP/doc.pdf", "normal key"},
		{"filing_pdf", "BHP.AU/report.pdf", "vire/filing_pdf/BHP.AU/report.pdf", "dots in key"},
		{"filing_pdf", "BHP/2025/01/01.pdf", "vire/filing_pdf/BHP/2025/01/01.pdf", "nested slashes"},
		{"chart", "port/chart.png", "vire/chart/port/chart.png", "different category"},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			got := store.objectKey(tc.category, tc.key)
			if got != tc.expected {
				t.Errorf("objectKey(%q, %q) = %q, want %q", tc.category, tc.key, got, tc.expected)
			}
		})
	}
}

// ============================================================================
// S3S-2. Object key without prefix
// ============================================================================

func TestS3Stress_ObjectKey_NoPrefix(t *testing.T) {
	store := &S3Store{prefix: ""}

	got := store.objectKey("filing_pdf", "BHP/doc.pdf")
	expected := "filing_pdf/BHP/doc.pdf"
	if got != expected {
		t.Errorf("objectKey without prefix = %q, want %q", got, expected)
	}
}

// ============================================================================
// S3S-3. Object key with adversarial inputs
// ============================================================================

func TestS3Stress_ObjectKey_Adversarial(t *testing.T) {
	store := &S3Store{prefix: "vire"}

	cases := []struct {
		category string
		key      string
		desc     string
	}{
		{"../escape", "file.pdf", "path traversal in category"},
		{"filing_pdf", "../../../etc/passwd", "path traversal in key"},
		{"filing_pdf", "key with spaces.pdf", "spaces"},
		{"filing_pdf", "key\x00null.pdf", "null byte"},
		{"filing_pdf", "very/" + longString(500) + ".pdf", "very long key segment"},
		{"", "file.pdf", "empty category"},
		{"filing_pdf", "", "empty key"},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			// objectKey should not panic regardless of input
			got := store.objectKey(tc.category, tc.key)
			if got == "" {
				t.Error("objectKey returned empty string")
			}
		})
	}
}

func longString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'x'
	}
	return string(b)
}
