package tune

import (
	"slices"

	"github.com/sunerpy/gpu-tools/internal/gpu"
)

type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
)

type Recommendation struct {
	Severity        Severity
	Title           string
	Detail          string
	SuggestedAction string
}

func Evaluate(d gpu.Device) []Recommendation {
	recommendations := make([]Recommendation, 0, 5)
	if hasPowerHeadroom(d) {
		recommendations = append(recommendations, Recommendation{
			Severity:        SeverityInfo,
			Title:           "Power headroom available",
			Detail:          "GPU is drawing less than 70% of its configured power limit.",
			SuggestedAction: "Consider scheduling additional workload or reviewing whether the configured power limit is higher than needed.",
		})
	}
	if d.ECCEnabled != nil && *d.ECCEnabled {
		recommendations = append(recommendations, Recommendation{
			Severity:        SeverityInfo,
			Title:           "ECC is enabled",
			Detail:          "ECC protects memory integrity and can reduce available memory bandwidth on some workloads.",
			SuggestedAction: "Keep ECC enabled for correctness-sensitive workloads; benchmark with policy approval before changing ECC settings outside gpu-tools.",
		})
	}
	if hasAnyThrottleReason(d.ThrottleReasons, "hw_thermal_slowdown", "sw_thermal_slowdown") {
		recommendations = append(recommendations, Recommendation{
			Severity:        SeverityWarning,
			Title:           "Thermal throttle active",
			Detail:          "GPU reports thermal slowdown throttle reasons.",
			SuggestedAction: "Inspect cooling, airflow, fan policy, and chassis intake temperature before increasing workload.",
		})
	}
	if hasAnyThrottleReason(d.ThrottleReasons, "sw_power_cap", "hw_power_brake_slowdown") {
		recommendations = append(recommendations, Recommendation{
			Severity:        SeverityWarning,
			Title:           "Power-cap throttle active",
			Detail:          "GPU reports throttling caused by a power cap or power brake condition.",
			SuggestedAction: "Review workload power demand and configured platform power policy; gpu-tools will not change the power limit.",
		})
	}
	if d.Temperature >= 85 {
		recommendations = append(recommendations, Recommendation{
			Severity:        SeverityWarning,
			Title:           "High temperature",
			Detail:          "GPU temperature is at or above 85°C.",
			SuggestedAction: "Reduce sustained load or improve cooling before the GPU reaches stronger thermal throttling.",
		})
	}
	return recommendations
}

func hasPowerHeadroom(d gpu.Device) bool {
	return d.PowerLimit > 0 && uint64(d.PowerDraw)*100 < uint64(d.PowerLimit)*70
}

func hasAnyThrottleReason(reasons []string, targets ...string) bool {
	for _, target := range targets {
		if slices.Contains(reasons, target) {
			return true
		}
	}
	return false
}
