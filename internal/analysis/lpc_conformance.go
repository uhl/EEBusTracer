package analysis

import (
	"fmt"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/spineparse"
)

// LPCVerdict captures the outcome of a single conformance check.
type LPCVerdict string

const (
	LPCPass          LPCVerdict = "pass"
	LPCFail          LPCVerdict = "fail"
	LPCNotApplicable LPCVerdict = "na"
	LPCInconclusive  LPCVerdict = "inconclusive"
)

// LPCEvidence is one offending or supporting datapoint backing a verdict.
type LPCEvidence struct {
	MessageIDs []int64    `json:"messageIds"`
	Note       string     `json:"note"`
	Timestamp  *time.Time `json:"timestamp,omitempty"`
}

// LPCCheckResult is the outcome of one LPC-TS-xxx requirement check.
type LPCCheckResult struct {
	RequirementID string        `json:"requirementId"`
	Title         string        `json:"title"`
	Description   string        `json:"description"`
	ATCRefs       []string      `json:"atcRefs"`
	Verdict       LPCVerdict    `json:"verdict"`
	Summary       string        `json:"summary"`
	Evidence      []LPCEvidence `json:"evidence"`
}

// LPCSummary aggregates verdict counts for the report header.
type LPCSummary struct {
	Total         int `json:"total"`
	Pass          int `json:"pass"`
	Fail          int `json:"fail"`
	NotApplicable int `json:"notApplicable"`
	Inconclusive  int `json:"inconclusive"`
}

// LPCConformanceReport is the top-level result of LPC trace evaluation.
type LPCConformanceReport struct {
	LPCDetected bool             `json:"lpcDetected"`
	Checks      []LPCCheckResult `json:"checks"`
	Summary     LPCSummary       `json:"summary"`
}

// LimitDesc is a minimal projection of LoadControl limit description data the
// LPC checks need. Kept here (not imported from api) to avoid a package cycle.
type LimitDesc struct {
	LimitID       string
	LimitCategory string
	ScopeType     string
}

// KeyValueDesc is a minimal projection of DeviceConfiguration key/value
// description data the LPC checks need.
type KeyValueDesc struct {
	KeyID     string
	KeyName   string
	ValueType string
}

// LPCConformanceInput is the input DTO for EvaluateLPCConformance.
type LPCConformanceInput struct {
	Messages      []*model.Message
	UseCases      []DeviceUseCases
	Connections   []ConnectionInfo
	LimitDescs    map[string]LimitDesc
	KeyValueDescs map[string]KeyValueDesc
}

// Spec-defined timings. Per the locked-in decision, the heartbeat threshold is
// strict 60s — any gap > 60.0s fails the check, even by milliseconds.
const lpcHeartbeatThreshold = 60 * time.Second

// LPC actor names used in nodeManagementUseCaseData announcements.
const (
	lpcActorEnergyGuard       = "energyGuard"
	lpcActorControllableSystem = "controllableSystem"
)

// lpcCtx caches pre-computed indices that every check shares.
type lpcCtx struct {
	input LPCConformanceInput
	// LPC role membership: set of device addresses that announced this role.
	egDevices map[string]bool
	csDevices map[string]bool
}

func newLPCCtx(input LPCConformanceInput) *lpcCtx {
	ctx := &lpcCtx{
		input:     input,
		egDevices: map[string]bool{},
		csDevices: map[string]bool{},
	}
	for _, duc := range input.UseCases {
		if !ucHasLPC(duc) {
			continue
		}
		switch duc.Actor {
		case lpcActorEnergyGuard:
			ctx.egDevices[duc.DeviceAddr] = true
		case lpcActorControllableSystem:
			ctx.csDevices[duc.DeviceAddr] = true
		}
	}
	return ctx
}

func ucHasLPC(duc DeviceUseCases) bool {
	for _, uc := range duc.UseCases {
		if uc.Abbreviation == "LPC" {
			return true
		}
	}
	return false
}

// EvaluateLPCConformance runs every passive LPC conformance check that the
// recorded trace makes observable. Returns LPCDetected=false with an empty
// check list when the trace contains no LPC participants.
func EvaluateLPCConformance(input LPCConformanceInput) LPCConformanceReport {
	report := LPCConformanceReport{
		Checks: []LPCCheckResult{},
	}

	if !detectLPC(input.UseCases) {
		return report
	}
	report.LPCDetected = true

	ctx := newLPCCtx(input)
	report.Checks = append(report.Checks,
		checkEGHeartbeat(ctx),
		checkCSHeartbeat(ctx),
		checkAPCLNonNegative(ctx),
	)

	report.Summary = summarizeChecks(report.Checks)
	return report
}

// summarizeChecks counts verdicts for the report header.
func summarizeChecks(checks []LPCCheckResult) LPCSummary {
	s := LPCSummary{Total: len(checks)}
	for _, c := range checks {
		switch c.Verdict {
		case LPCPass:
			s.Pass++
		case LPCFail:
			s.Fail++
		case LPCNotApplicable:
			s.NotApplicable++
		case LPCInconclusive:
			s.Inconclusive++
		}
	}
	return s
}

// checkHeartbeatFor evaluates the heartbeat-interval requirement for a set of
// sender devices. roleLabel ("EG"/"CS") is used in the NA summary message.
func checkHeartbeatFor(ctx *lpcCtx, roleDevices map[string]bool, roleLabel, requirementID, title, description string, atcRefs []string) LPCCheckResult {
	result := LPCCheckResult{
		RequirementID: requirementID,
		Title:         title,
		Description:   description,
		ATCRefs:       atcRefs,
		Evidence:      []LPCEvidence{},
	}

	if len(roleDevices) == 0 {
		result.Verdict = LPCNotApplicable
		result.Summary = "No device announced LPC actor=" + roleLabel
		return result
	}

	pairOrder, gaps := collectHeartbeatGapsByPair(ctx.input.Messages)

	totalHeartbeats := 0
	for _, msg := range ctx.input.Messages {
		if msg.FunctionSet != "DeviceDiagnosisHeartbeatData" {
			continue
		}
		if roleDevices[msg.DeviceSource] {
			totalHeartbeats++
		}
	}

	var violations []LPCEvidence
	maxGap := time.Duration(0)
	gapCount := 0

	for _, pair := range pairOrder {
		if !roleDevices[pair.a] {
			continue
		}
		for _, g := range gaps[pair] {
			gapCount++
			if g.Duration > maxGap {
				maxGap = g.Duration
			}
			if g.Duration > lpcHeartbeatThreshold {
				ts := g.To
				violations = append(violations, LPCEvidence{
					MessageIDs: []int64{g.FromID, g.ToID},
					Note:       formatGapNote(g),
					Timestamp:  &ts,
				})
			}
		}
	}

	switch {
	case totalHeartbeats == 0:
		result.Verdict = LPCNotApplicable
		result.Summary = "No " + roleLabel + " heartbeats observed"
	case gapCount == 0:
		result.Verdict = LPCInconclusive
		result.Summary = "Only 1 " + roleLabel + " heartbeat observed — cannot measure intervals"
	case len(violations) == 0:
		result.Verdict = LPCPass
		result.Summary = formatHeartbeatPassSummary(roleLabel, gapCount, maxGap)
	default:
		result.Verdict = LPCFail
		result.Summary = formatHeartbeatFailSummary(roleLabel, len(violations), gapCount, maxGap)
		result.Evidence = violations
	}
	return result
}

// checkEGHeartbeat covers LPC-TS-006: EG heartbeats must arrive at least every 60s.
func checkEGHeartbeat(ctx *lpcCtx) LPCCheckResult {
	return checkHeartbeatFor(
		ctx,
		ctx.egDevices,
		"EG",
		"LPC-TS-006",
		"EG heartbeat interval",
		"The heartbeat of the EG SHALL be sent at least every 60 seconds.",
		[]string{"ATC_COM_PT_EGHeartbeat_001"},
	)
}

// apclDescriptor extracts limit values from LoadControlLimitListData writes.
var apclDescriptor = spineparse.ExtractionDescriptor{
	CmdKey:       "loadControlLimitListData",
	DataArrayKey: "loadControlLimitData",
	IDField:      "limitId",
}

// checkAPCLNonNegative covers LPC-TS-001: every APCL value SHALL be >= 0.
// Per locked-in decision, this flags negative values regardless of isLimitActive.
func checkAPCLNonNegative(ctx *lpcCtx) LPCCheckResult {
	result := LPCCheckResult{
		RequirementID: "LPC-TS-001",
		Title:         "APCL value non-negative",
		Description:   "The Active Power Consumption Limit SHALL always be greater than or equal to zero.",
		ATCRefs: []string{
			"ATC_COM_PT_EGMessages_001",
			"ATC_COM_PT_EGMessages_003",
			"ATC_COM_PT_CSConnection_007",
			"ATC_COM_PT_CSConnection_008",
		},
		Evidence: []LPCEvidence{},
	}

	itemCount := 0
	var violations []LPCEvidence
	for _, msg := range ctx.input.Messages {
		if msg.FunctionSet != "LoadControlLimitListData" || msg.CmdClassifier != "write" {
			continue
		}
		for _, item := range spineparse.ExtractGenericData(msg.SpinePayload, apclDescriptor) {
			itemCount++
			if item.Value < 0 {
				ts := msg.Timestamp
				violations = append(violations, LPCEvidence{
					MessageIDs: []int64{msg.ID},
					Note:       fmt.Sprintf("APCL limitId=%s value=%.1fW < 0", item.ID, item.Value),
					Timestamp:  &ts,
				})
			}
		}
	}

	switch {
	case itemCount == 0:
		result.Verdict = LPCNotApplicable
		result.Summary = "No APCL write commands observed"
	case len(violations) == 0:
		result.Verdict = LPCPass
		result.Summary = fmt.Sprintf("All %d APCL write value(s) ≥ 0", itemCount)
	default:
		result.Verdict = LPCFail
		result.Summary = fmt.Sprintf("%d/%d APCL write value(s) below 0", len(violations), itemCount)
		result.Evidence = violations
	}
	return result
}

// checkCSHeartbeat covers LPC-TS-007: CS heartbeats must arrive at least every 60s.
func checkCSHeartbeat(ctx *lpcCtx) LPCCheckResult {
	return checkHeartbeatFor(
		ctx,
		ctx.csDevices,
		"CS",
		"LPC-TS-007",
		"CS heartbeat interval",
		"The heartbeat of the CS SHALL be sent at least every 60 seconds.",
		[]string{"ATC_COM_PT_CSHeartbeat_001"},
	)
}

func formatGapNote(g heartbeatGap) string {
	return fmt.Sprintf("%.3fs gap between heartbeats", g.Duration.Seconds())
}

func formatHeartbeatPassSummary(role string, count int, maxGap time.Duration) string {
	return fmt.Sprintf("%d %s heartbeat interval(s), max %.3fs", count, role, maxGap.Seconds())
}

func formatHeartbeatFailSummary(role string, viol, count int, maxGap time.Duration) string {
	return fmt.Sprintf("%d/%d %s heartbeat gap(s) exceeded 60s (worst %.3fs)", viol, count, role, maxGap.Seconds())
}

// detectLPC returns true if any device announced the LPC use case.
func detectLPC(useCases []DeviceUseCases) bool {
	for _, duc := range useCases {
		for _, uc := range duc.UseCases {
			if uc.Abbreviation == "LPC" {
				return true
			}
		}
	}
	return false
}
