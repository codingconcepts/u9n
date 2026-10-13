package render

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/codingconcepts/u9n/internal/estate"
)

func TestJSON(t *testing.T) {
	var b strings.Builder
	if err := JSON(&b, fixture(estate.Assumptions{Headroom: 20, Years: 1})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out jsonSummary
	if err := json.Unmarshal([]byte(b.String()), &out); err != nil {
		t.Fatalf("decoding the output: %v", err)
	}

	if out.Clusters != 1 || out.ProvisionedVCPU != 10 || out.HeadroomPercent != 20 {
		t.Errorf("unexpected estate: %+v", out)
	}

	if !out.Windowed || out.PeakDiversityPercent == nil || *out.PeakDiversityPercent != 0 {
		t.Errorf("expected a windowed estate with its diversity reported, got %+v", out)
	}

	if len(out.Scenarios) != 3 {
		t.Fatalf("expected 3 scenarios, got %d", len(out.Scenarios))
	}

	// Only the hour aware sizing has peak hours to report, and it must say so
	// with null rather than a misleading midnight.
	for _, sc := range out.Scenarios {
		switch sc.Key {
		case estate.KeyHourAware:
			if !slices.Equal(sc.PeakHours, []int{9, 10}) {
				t.Errorf("hour aware: expected peak hours 9 and 10, got %v", sc.PeakHours)
			}
		default:
			if sc.PeakHours != nil {
				t.Errorf("%s: expected no peak hours, got %v", sc.Key, sc.PeakHours)
			}
		}
	}

	if len(out.Hourly) != estate.HoursPerDay {
		t.Fatalf("expected %d hours, got %d", estate.HoursPerDay, len(out.Hourly))
	}

	if out.Hourly[9].VCPU != 5 || out.Hourly[0].VCPU != 1 {
		t.Errorf("unexpected hourly profile: %+v", out.Hourly)
	}

	if len(out.ClusterDetail) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(out.ClusterDetail))
	}

	detail := out.ClusterDetail[0]
	if detail.Name != "a" || detail.PeakHourContribution != 5 {
		t.Errorf("unexpected cluster detail: %+v", detail)
	}

	if detail.PeakFrom == nil || *detail.PeakFrom != 9 || detail.PeakTo == nil || *detail.PeakTo != 11 {
		t.Errorf("expected the peak window to be reported, got %+v", detail)
	}
}

// A cluster without a peak window has no window to report, and an estate
// without one has no diversity.
func TestJSONWithoutWindows(t *testing.T) {
	s := estate.Summarise([]estate.Cluster{{Name: "a", Provisioned: 10, Peak: 5}}, estate.Assumptions{Headroom: 20, Years: 1})

	var b strings.Builder
	if err := JSON(&b, s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out jsonSummary
	if err := json.Unmarshal([]byte(b.String()), &out); err != nil {
		t.Fatalf("decoding the output: %v", err)
	}

	if out.PeakDiversityPercent != nil {
		t.Errorf("expected no diversity, got %g", *out.PeakDiversityPercent)
	}

	if len(out.Scenarios) != 3 {
		t.Fatalf("expected 3 scenarios, got %d", len(out.Scenarios))
	}

	// The hour aware sizing repeats the worst case, but still answers the peak
	// hours rather than leaving them null. Every hour is as busy as the last,
	// so every hour is a peak.
	if hourAware := out.Scenarios[2]; len(hourAware.PeakHours) != estate.HoursPerDay {
		t.Errorf("expected the hour aware sizing to peak all day, got %+v", hourAware)
	}

	if detail := out.ClusterDetail[0]; detail.PeakFrom != nil || detail.PeakTo != nil {
		t.Errorf("expected no peak window, got %+v", detail)
	}
}

func TestJSONReportsWriteErrors(t *testing.T) {
	if err := JSON(failingWriter{}, fixture(estate.Assumptions{Headroom: 20, Years: 1})); err == nil {
		t.Fatal("expected an error from a writer that refuses to write")
	}
}

func TestProfileJSON(t *testing.T) {
	var b strings.Builder
	if err := ProfileJSON(&b, profileFixture, Bands{Base: true, Peak: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out jsonProfile
	if err := json.Unmarshal([]byte(b.String()), &out); err != nil {
		t.Fatalf("decoding the output: %v", err)
	}

	if !out.Bands.Base || !out.Bands.Peak {
		t.Errorf("expected both bands to be reported, got %+v", out.Bands)
	}

	if out.PeakVCPU != 5 || !slices.Equal(out.PeakHours, []int{9, 10}) {
		t.Errorf("expected a peak of 5 at 09:00 and 10:00, got %g at %v", out.PeakVCPU, out.PeakHours)
	}

	if len(out.Hourly) != estate.HoursPerDay {
		t.Fatalf("expected %d hours, got %d", estate.HoursPerDay, len(out.Hourly))
	}

	// The base is the same in all 24 hours and the uplift only lands in the
	// two the cluster peaks in.
	if exp := (jsonProfileHour{Hour: 0, BaseVCPU: 1, VCPU: 1}); out.Hourly[0] != exp {
		t.Errorf("expected an off peak hour of %+v, got %+v", exp, out.Hourly[0])
	}

	if exp := (jsonProfileHour{Hour: 9, BaseVCPU: 1, UpliftVCPU: 4, VCPU: 5}); out.Hourly[9] != exp {
		t.Errorf("expected a peak hour of %+v, got %+v", exp, out.Hourly[9])
	}
}

// Dropping a band zeroes it everywhere rather than leaving it in the figures
// the chart isn't drawing.
func TestProfileJSONReportsOnlyTheBandsAskedFor(t *testing.T) {
	var b strings.Builder
	if err := ProfileJSON(&b, profileFixture, Bands{Peak: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out jsonProfile
	if err := json.Unmarshal([]byte(b.String()), &out); err != nil {
		t.Fatalf("decoding the output: %v", err)
	}

	if out.Bands.Base {
		t.Errorf("expected no base band, got %+v", out.Bands)
	}

	if exp := (jsonProfileHour{Hour: 9, UpliftVCPU: 4, VCPU: 4}); out.Hourly[9] != exp {
		t.Errorf("expected the uplift alone, got %+v", out.Hourly[9])
	}
}

func TestProfileJSONRejectsADocumentWithNoBands(t *testing.T) {
	var b strings.Builder

	if err := ProfileJSON(&b, profileFixture, Bands{}); !errors.Is(err, errNoBands) {
		t.Fatalf("expected the empty profile to be refused, got %v", err)
	}

	if b.String() != "" {
		t.Errorf("expected nothing to be written, got:\n%s", b.String())
	}
}

func TestProfileJSONReportsWriteErrors(t *testing.T) {
	if err := ProfileJSON(failingWriter{}, profileFixture, Bands{Base: true, Peak: true}); err == nil {
		t.Fatal("expected an error from a writer that refuses to write")
	}
}
