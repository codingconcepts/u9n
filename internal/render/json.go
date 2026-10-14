package render

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/codingconcepts/u9n/internal/estate"
)

type jsonSummary struct {
	Clusters             int            `json:"clusters"`
	ProvisionedVCPU      float64        `json:"provisioned_vcpu"`
	HeadroomPercent      float64        `json:"headroom_percent"`
	GrowthPercent        float64        `json:"growth_percent"`
	Years                float64        `json:"years"`
	GrowthFactor         float64        `json:"growth_factor"`
	Windowed             bool           `json:"windowed"`
	PeakDiversityPercent *float64       `json:"peak_diversity_percent"`
	Scenarios            []jsonScenario `json:"scenarios"`
	Hourly               []jsonHour     `json:"hourly"`
	ClusterDetail        []jsonCluster  `json:"cluster_detail"`
}

type jsonScenario struct {
	Key                string  `json:"key"`
	Label              string  `json:"label"`
	PeakVCPU           float64 `json:"peak_vcpu"`
	UtilisationPercent float64 `json:"utilisation_percent"`
	PeakHours          []int   `json:"peak_hours"`
	RequiredVCPU       float64 `json:"required_vcpu"`
	SavedVCPU          float64 `json:"saved_vcpu"`
	SavedPercent       float64 `json:"saved_percent"`
}

type jsonHour struct {
	Hour int     `json:"hour"`
	VCPU float64 `json:"vcpu"`
}

type jsonProfile struct {
	Bands     jsonBands         `json:"bands"`
	PeakVCPU  float64           `json:"peak_vcpu"`
	PeakHours []int             `json:"peak_hours"`
	Hourly    []jsonProfileHour `json:"hourly"`
}

type jsonBands struct {
	Base bool `json:"base"`
	Peak bool `json:"peak"`
}

// jsonProfileHour is one hour split the way the chart stacks it. A band that
// wasn't asked for reads zero, the same as it is drawn, so the vcpu is always
// the sum of the two beside it.
type jsonProfileHour struct {
	Hour       int     `json:"hour"`
	BaseVCPU   float64 `json:"base_vcpu"`
	UpliftVCPU float64 `json:"uplift_vcpu"`
	VCPU       float64 `json:"vcpu"`
}

type jsonCluster struct {
	Name                 string  `json:"name"`
	ProvisionedVCPU      float64 `json:"provisioned_vcpu"`
	PeakVCPU             float64 `json:"peak_vcpu"`
	BaseVCPU             float64 `json:"base_vcpu"`
	PeakFrom             *int    `json:"peak_from"`
	PeakTo               *int    `json:"peak_to"`
	PeakHourContribution float64 `json:"peak_hour_contribution_vcpu"`
}

// JSON writes the same figures as the table plus per-cluster detail, for
// piping into other tools.
func JSON(w io.Writer, s estate.Summary) error {
	out := jsonSummary{
		Clusters:        len(s.Clusters),
		ProvisionedVCPU: s.Provisioned,
		HeadroomPercent: s.Headroom,
		GrowthPercent:   s.Growth,
		Years:           s.Years,
		GrowthFactor:    s.GrowthFactor,
		Windowed:        s.Windowed,
	}

	if s.Windowed {
		diversity := s.Diversity()
		out.PeakDiversityPercent = &diversity
	}

	for _, sc := range s.Scenarios() {
		js := jsonScenario{
			Key:                sc.Key,
			Label:              sc.Label,
			PeakVCPU:           sc.Peak,
			UtilisationPercent: sc.Utilisation(),
			RequiredVCPU:       sc.Required,
			SavedVCPU:          sc.Saved(),
			SavedPercent:       sc.SavedPercent(),
		}

		if sc.Hourly {
			js.PeakHours = sc.PeakHours
		}

		out.Scenarios = append(out.Scenarios, js)
	}

	for hour, total := range s.Hourly {
		out.Hourly = append(out.Hourly, jsonHour{Hour: hour, VCPU: total})
	}

	for _, c := range s.Clusters {
		jc := jsonCluster{
			Name:            c.Name,
			ProvisionedVCPU: c.Provisioned,
			PeakVCPU:        c.Peak,
			BaseVCPU:        c.Base,
			// Every peak hour draws the same total, but not the same mix of
			// clusters, so the contribution is read off the first of them.
			PeakHourContribution: c.VCPUAt(s.PeakOfSum.PeakHours[0]),
		}

		if c.Windowed {
			jc.PeakFrom, jc.PeakTo = &c.PeakFrom, &c.PeakTo
		}

		out.ClusterDetail = append(out.ClusterDetail, jc)
	}

	return encodeJSON(w, out)
}

// ProfileJSON writes the hourly profile the chart draws, for piping into other
// tools. It carries the bands it was asked for, since dropping one changes
// every figure under it.
func ProfileJSON(w io.Writer, clusters []estate.Cluster, bands Bands) error {
	if !bands.Base && !bands.Peak {
		return errNoBands
	}

	hours, top := profileBars(clusters, bands)

	out := jsonProfile{
		Bands:     jsonBands{Base: bands.Base, Peak: bands.Peak},
		PeakVCPU:  top,
		PeakHours: profilePeakHours(hours, top),
		Hourly:    make([]jsonProfileHour, 0, estate.HoursPerDay),
	}

	for hour, d := range hours {
		out.Hourly = append(out.Hourly, jsonProfileHour{
			Hour:       hour,
			BaseVCPU:   d.Base,
			UpliftVCPU: d.Uplift,
			VCPU:       d.Total(),
		})
	}

	return encodeJSON(w, out)
}

func encodeJSON(w io.Writer, out any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(out); err != nil {
		return fmt.Errorf("encoding json: %w", err)
	}

	return nil
}
