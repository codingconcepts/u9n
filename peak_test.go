package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPeak(t *testing.T) {
	config := testConfig(t)

	t.Run("charts both sizings against each other", func(t *testing.T) {
		var b strings.Builder
		if err := runPeak(&b, peakOptions{config: config}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for _, want := range []string{"Sum of the peaks", "Peak of the sum", "Avoided", "a", "09:00"} {
			if !strings.Contains(b.String(), want) {
				t.Errorf("expected the chart to contain %q, got:\n%s", want, b.String())
			}
		}
	})

	// The chart draws demand rather than a sizing, so nothing it prints should
	// carry the headroom the size command adds.
	t.Run("the peaks are the ones the config reports", func(t *testing.T) {
		var b strings.Builder
		if err := runPeak(&b, peakOptions{config: config}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if strings.Contains(b.String(), "6") {
			t.Errorf("expected the peak of 5 rather than a sizing of 6, got:\n%s", b.String())
		}
	})

	t.Run("unreadable config", func(t *testing.T) {
		var b strings.Builder

		err := runPeak(&b, peakOptions{config: filepath.Join(t.TempDir(), "missing.csv")})

		if err == nil || !strings.HasPrefix(err.Error(), "reading config: ") {
			t.Fatalf("expected the error to say which step failed, got %v", err)
		}
	})
}
