package analysis

import (
	"fmt"
	"strings"
)

// UseCaseLifecycleSpec defines what subscriptions and bindings a use case
// requires. Adding a new use case's lifecycle tracking requires only adding
// an entry to UseCaseLifecycleSpecs — no handler or frontend changes needed.
type UseCaseLifecycleSpec struct {
	// RequiredSubscriptions lists server feature types that need active subscriptions.
	// Uses SPINE feature type naming (e.g., "LoadControl").
	// Empty slice means subscriptions are N/A for this use case.
	RequiredSubscriptions []string

	// RequiredBindings lists server feature types that need active bindings.
	// Empty slice means bindings are N/A for this use case.
	RequiredBindings []string
}

// UseCaseLifecycleSpecs maps use case abbreviations to their lifecycle specs.
var UseCaseLifecycleSpecs = map[string]UseCaseLifecycleSpec{
	"LPC":   {RequiredSubscriptions: []string{"LoadControl"}, RequiredBindings: []string{"LoadControl"}},
	"LPP":   {RequiredSubscriptions: []string{"LoadControl"}, RequiredBindings: []string{"LoadControl"}},
	"MPC":   {RequiredSubscriptions: []string{"Measurement"}, RequiredBindings: []string{}},
	"MGCP":  {RequiredSubscriptions: []string{"Measurement"}, RequiredBindings: []string{}},
	"EVCC":  {RequiredSubscriptions: []string{"DeviceConfiguration"}, RequiredBindings: []string{}},
	"OPEV":  {RequiredSubscriptions: []string{"LoadControl"}, RequiredBindings: []string{"LoadControl"}},
	"DBEVC": {RequiredSubscriptions: []string{"Setpoint"}, RequiredBindings: []string{"Setpoint"}},
	"MOB":   {RequiredSubscriptions: []string{"Measurement"}, RequiredBindings: []string{}},
	"MOI":   {RequiredSubscriptions: []string{"Measurement"}, RequiredBindings: []string{}},
	"MOPVS": {RequiredSubscriptions: []string{"Measurement"}, RequiredBindings: []string{}},
}

// StepStatus represents the status of a lifecycle step.
type StepStatus string

const (
	StepPass    StepStatus = "pass"
	StepFail    StepStatus = "fail"
	StepPartial StepStatus = "partial"
	StepNA      StepStatus = "na"
	StepPending StepStatus = "pending"
)

// LifecycleStep represents a single step in the use case lifecycle checklist.
type LifecycleStep struct {
	Name    string     `json:"name"`
	Status  StepStatus `json:"status"`
	Details string     `json:"details"`
}

// DeviceUseCaseLifecycle is the lifecycle checklist result for one device+UC pair.
type DeviceUseCaseLifecycle struct {
	DeviceAddr    string          `json:"deviceAddr"`
	ShortName     string          `json:"shortName"`
	UseCaseAbbr   string          `json:"useCaseAbbr"`
	UseCaseName   string          `json:"useCaseName"`
	Available     bool            `json:"available"`
	Steps         []LifecycleStep `json:"steps"`
	OverallStatus StepStatus      `json:"overallStatus"`
}

// ConnectionInfo carries SHIP connection state for lifecycle evaluation.
type ConnectionInfo struct {
	DeviceSource string
	DeviceDest   string
	CurrentState string
}

// LifecycleInput is the input DTO for EvaluateLifecycles, keeping the analysis
// package free of circular imports with the api package.
type LifecycleInput struct {
	Connections   []ConnectionInfo
	Devices       []DeviceInfo
	UseCases      []DeviceUseCases
	Subscriptions []SubscriptionEntry
	Bindings      []BindingEntry
}

// EvaluateLifecycles evaluates the 5-step lifecycle checklist for every
// device+UC pair found in the input.
func EvaluateLifecycles(input LifecycleInput) []DeviceUseCaseLifecycle {
	results := []DeviceUseCaseLifecycle{}

	for _, duc := range input.UseCases {
		for _, uc := range duc.UseCases {
			spec, hasSpec := UseCaseLifecycleSpecs[uc.Abbreviation]

			steps := []LifecycleStep{
				evaluateHandshake(duc.DeviceAddr, input.Connections),
				evaluateDiscovery(duc.DeviceAddr, uc.Abbreviation, input.Devices),
				evaluateAnnounced(uc),
				evaluateSubscriptions(duc.DeviceAddr, spec, hasSpec, input.Subscriptions),
				evaluateBindings(duc.DeviceAddr, spec, hasSpec, input.Bindings),
			}

			results = append(results, DeviceUseCaseLifecycle{
				DeviceAddr:    duc.DeviceAddr,
				ShortName:     shortDeviceAddr(duc.DeviceAddr),
				UseCaseAbbr:   uc.Abbreviation,
				UseCaseName:   uc.UseCaseName,
				Available:     uc.Available,
				Steps:         steps,
				OverallStatus: computeOverallStatus(steps),
			})
		}
	}

	return results
}

func evaluateHandshake(deviceAddr string, connections []ConnectionInfo) LifecycleStep {
	step := LifecycleStep{Name: "SHIP Handshake"}

	for _, c := range connections {
		if c.DeviceSource != deviceAddr && c.DeviceDest != deviceAddr {
			continue
		}
		if c.CurrentState == "data" {
			step.Status = StepPass
			step.Details = "Connection reached data state"
			return step
		}
		// Connection exists but not at data state
		step.Status = StepFail
		step.Details = fmt.Sprintf("Connection at %q state", c.CurrentState)
		return step
	}

	step.Status = StepPending
	step.Details = "No connection data observed"
	return step
}

func evaluateDiscovery(deviceAddr, ucAbbr string, devices []DeviceInfo) LifecycleStep {
	step := LifecycleStep{Name: "Feature Discovery"}

	spec, ok := UseCaseFunctionSets[ucAbbr]
	if !ok || len(spec.Functions) == 0 {
		step.Status = StepPending
		step.Details = "No function set mapping for this use case"
		return step
	}

	// Find device in discovery data, collecting only functions from
	// entities matching the UC's entity type constraint.
	var deviceFound bool
	var allFunctions []string
	for _, dev := range devices {
		if dev.DeviceAddr != deviceAddr {
			continue
		}
		deviceFound = true
		for _, ent := range dev.Entities {
			if !matchesEntityType(ent.EntityType, spec.EntityTypes) {
				continue
			}
			for _, feat := range ent.Features {
				allFunctions = append(allFunctions, feat.Functions...)
			}
		}
	}

	if !deviceFound {
		step.Status = StepPending
		step.Details = "Device not found in discovery data"
		return step
	}

	// Check how many required functions are present (case-insensitive)
	funcSet := make(map[string]bool, len(allFunctions))
	for _, f := range allFunctions {
		funcSet[strings.ToLower(f)] = true
	}

	found := 0
	var missing []string
	for _, req := range spec.Functions {
		if funcSet[strings.ToLower(req)] {
			found++
		} else {
			missing = append(missing, req)
		}
	}

	switch {
	case found == len(spec.Functions):
		step.Status = StepPass
		step.Details = fmt.Sprintf("All %d function sets discovered", found)
	case found > 0:
		step.Status = StepPartial
		step.Details = fmt.Sprintf("%d of %d function sets discovered; missing: %s", found, len(spec.Functions), strings.Join(missing, ", "))
	default:
		step.Status = StepFail
		step.Details = fmt.Sprintf("0 of %d function sets discovered; missing: %s", len(spec.Functions), strings.Join(missing, ", "))
	}

	return step
}

func evaluateAnnounced(uc UseCaseInfo) LifecycleStep {
	step := LifecycleStep{Name: "UC Announced"}

	if uc.Available {
		step.Status = StepPass
		step.Details = "Use case available"
	} else {
		step.Status = StepFail
		step.Details = "Use case announced but not available"
	}

	return step
}

func evaluateSubscriptions(deviceAddr string, spec UseCaseLifecycleSpec, hasSpec bool, subscriptions []SubscriptionEntry) LifecycleStep {
	step := LifecycleStep{Name: "Subscriptions"}

	if !hasSpec || len(spec.RequiredSubscriptions) == 0 {
		step.Status = StepNA
		step.Details = "No subscriptions required"
		return step
	}

	// Find active subscriptions for this device's feature types
	activeTypes := make(map[string]bool)
	for _, sub := range subscriptions {
		if sub.ServerDevice == deviceAddr && sub.Active && sub.ServerFeatureType != "" {
			activeTypes[sub.ServerFeatureType] = true
		}
	}

	found := 0
	var missing []string
	for _, req := range spec.RequiredSubscriptions {
		if activeTypes[req] {
			found++
		} else {
			missing = append(missing, req)
		}
	}

	switch {
	case found == len(spec.RequiredSubscriptions):
		step.Status = StepPass
		step.Details = fmt.Sprintf("All %d subscriptions active", found)
	case found > 0:
		step.Status = StepPartial
		step.Details = fmt.Sprintf("%d of %d subscriptions active; missing: %s", found, len(spec.RequiredSubscriptions), strings.Join(missing, ", "))
	default:
		step.Status = StepFail
		step.Details = fmt.Sprintf("0 of %d subscriptions active; missing: %s", len(spec.RequiredSubscriptions), strings.Join(missing, ", "))
	}

	return step
}

func evaluateBindings(deviceAddr string, spec UseCaseLifecycleSpec, hasSpec bool, bindings []BindingEntry) LifecycleStep {
	step := LifecycleStep{Name: "Bindings"}

	if !hasSpec || len(spec.RequiredBindings) == 0 {
		step.Status = StepNA
		step.Details = "No bindings required"
		return step
	}

	// Find active bindings for this device's feature types
	activeTypes := make(map[string]bool)
	for _, b := range bindings {
		if b.ServerDevice == deviceAddr && b.Active && b.ServerFeatureType != "" {
			activeTypes[b.ServerFeatureType] = true
		}
	}

	found := 0
	var missing []string
	for _, req := range spec.RequiredBindings {
		if activeTypes[req] {
			found++
		} else {
			missing = append(missing, req)
		}
	}

	switch {
	case found == len(spec.RequiredBindings):
		step.Status = StepPass
		step.Details = fmt.Sprintf("All %d bindings active", found)
	case found > 0:
		step.Status = StepPartial
		step.Details = fmt.Sprintf("%d of %d bindings active; missing: %s", found, len(spec.RequiredBindings), strings.Join(missing, ", "))
	default:
		step.Status = StepFail
		step.Details = fmt.Sprintf("0 of %d bindings active; missing: %s", len(spec.RequiredBindings), strings.Join(missing, ", "))
	}

	return step
}

// computeOverallStatus returns the worst non-NA step status.
// Priority: fail > partial > pending > pass.
func computeOverallStatus(steps []LifecycleStep) StepStatus {
	worst := StepPass
	for _, s := range steps {
		if s.Status == StepNA {
			continue
		}
		if statusPriority(s.Status) > statusPriority(worst) {
			worst = s.Status
		}
	}
	return worst
}

func statusPriority(s StepStatus) int {
	switch s {
	case StepPass:
		return 0
	case StepPending:
		return 1
	case StepPartial:
		return 2
	case StepFail:
		return 3
	default:
		return -1
	}
}
