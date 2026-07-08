package tune

import (
	"reflect"
	"testing"

	"github.com/sunerpy/gpu-tools/internal/gpu"
)

func TestEvaluate_returnsRecommendations_whenEachRuleConditionHolds(t *testing.T) {
	eccEnabled := true
	tests := []struct {
		name   string
		device gpu.Device
		want   []Recommendation
	}{
		{
			name: "power headroom below seventy percent of limit",
			device: gpu.Device{
				PowerDraw:  100_000,
				PowerLimit: 200_000,
			},
			want: []Recommendation{{
				Severity:        SeverityInfo,
				Title:           "Power headroom available",
				Detail:          "GPU is drawing less than 70% of its configured power limit.",
				SuggestedAction: "Consider scheduling additional workload or reviewing whether the configured power limit is higher than needed.",
			}},
		},
		{
			name: "ecc enabled",
			device: gpu.Device{
				ECCEnabled: &eccEnabled,
			},
			want: []Recommendation{{
				Severity:        SeverityInfo,
				Title:           "ECC is enabled",
				Detail:          "ECC protects memory integrity and can reduce available memory bandwidth on some workloads.",
				SuggestedAction: "Keep ECC enabled for correctness-sensitive workloads; benchmark with policy approval before changing ECC settings outside gpu-tools.",
			}},
		},
		{
			name: "hardware thermal throttle",
			device: gpu.Device{
				ThrottleReasons: []string{"hw_thermal_slowdown"},
			},
			want: []Recommendation{{
				Severity:        SeverityWarning,
				Title:           "Thermal throttle active",
				Detail:          "GPU reports thermal slowdown throttle reasons.",
				SuggestedAction: "Inspect cooling, airflow, fan policy, and chassis intake temperature before increasing workload.",
			}},
		},
		{
			name: "software thermal throttle",
			device: gpu.Device{
				ThrottleReasons: []string{"sw_thermal_slowdown"},
			},
			want: []Recommendation{{
				Severity:        SeverityWarning,
				Title:           "Thermal throttle active",
				Detail:          "GPU reports thermal slowdown throttle reasons.",
				SuggestedAction: "Inspect cooling, airflow, fan policy, and chassis intake temperature before increasing workload.",
			}},
		},
		{
			name: "software power cap throttle",
			device: gpu.Device{
				ThrottleReasons: []string{"sw_power_cap"},
			},
			want: []Recommendation{{
				Severity:        SeverityWarning,
				Title:           "Power-cap throttle active",
				Detail:          "GPU reports throttling caused by a power cap or power brake condition.",
				SuggestedAction: "Review workload power demand and configured platform power policy; gpu-tools will not change the power limit.",
			}},
		},
		{
			name: "hardware power brake throttle",
			device: gpu.Device{
				ThrottleReasons: []string{"hw_power_brake_slowdown"},
			},
			want: []Recommendation{{
				Severity:        SeverityWarning,
				Title:           "Power-cap throttle active",
				Detail:          "GPU reports throttling caused by a power cap or power brake condition.",
				SuggestedAction: "Review workload power demand and configured platform power policy; gpu-tools will not change the power limit.",
			}},
		},
		{
			name: "high temperature at threshold",
			device: gpu.Device{
				Temperature: 85,
			},
			want: []Recommendation{{
				Severity:        SeverityWarning,
				Title:           "High temperature",
				Detail:          "GPU temperature is at or above 85°C.",
				SuggestedAction: "Reduce sustained load or improve cooling before the GPU reaches stronger thermal throttling.",
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			got := Evaluate(tt.device)

			// Then
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("recommendations mismatch\nwant: %#v\n got: %#v", tt.want, got)
			}
		})
	}
}

func TestEvaluate_returnsNoRecommendations_whenRuleConditionsDoNotHold(t *testing.T) {
	eccDisabled := false
	tests := []struct {
		name   string
		device gpu.Device
	}{
		{name: "empty device"},
		{name: "power draw equals seventy percent", device: gpu.Device{PowerDraw: 70_000, PowerLimit: 100_000}},
		{name: "power limit missing", device: gpu.Device{PowerDraw: 1, PowerLimit: 0}},
		{name: "ecc disabled", device: gpu.Device{ECCEnabled: &eccDisabled}},
		{name: "ecc unknown", device: gpu.Device{ECCEnabled: nil}},
		{name: "unrelated throttle reason", device: gpu.Device{ThrottleReasons: []string{"gpu_idle"}}},
		{name: "temperature below threshold", device: gpu.Device{Temperature: 84}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			got := Evaluate(tt.device)

			// Then
			if len(got) != 0 {
				t.Fatalf("expected no recommendations, got %#v", got)
			}
		})
	}
}

func TestEvaluate_returnsRecommendationsInDeterministicOrder_whenMultipleRulesHold(t *testing.T) {
	eccEnabled := true
	device := gpu.Device{
		PowerDraw:       100_000,
		PowerLimit:      200_000,
		ECCEnabled:      &eccEnabled,
		ThrottleReasons: []string{"sw_power_cap", "hw_thermal_slowdown"},
		Temperature:     90,
	}

	// When
	got := Evaluate(device)

	// Then
	wantTitles := []string{
		"Power headroom available",
		"ECC is enabled",
		"Thermal throttle active",
		"Power-cap throttle active",
		"High temperature",
	}
	if len(got) != len(wantTitles) {
		t.Fatalf("expected %d recommendations, got %#v", len(wantTitles), got)
	}
	for i, want := range wantTitles {
		if got[i].Title != want {
			t.Fatalf("recommendation %d title mismatch: want %q got %q", i, want, got[i].Title)
		}
	}
}
