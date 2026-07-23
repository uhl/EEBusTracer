package analysis

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
)

func TestEvaluateLPCConformance_Empty(t *testing.T) {
	report := EvaluateLPCConformance(LPCConformanceInput{})
	if report.LPCDetected {
		t.Error("LPCDetected = true, want false for empty input")
	}
	if report.Checks == nil {
		t.Error("Checks = nil, want non-nil empty slice (JSON contract)")
	}
	if len(report.Checks) != 0 {
		t.Errorf("Checks len = %d, want 0", len(report.Checks))
	}
}

func TestEvaluateLPCConformance_NoLPC(t *testing.T) {
	input := LPCConformanceInput{
		UseCases: []DeviceUseCases{
			{DeviceAddr: "devA", UseCases: []UseCaseInfo{
				{Abbreviation: "MPC", Available: true},
			}},
		},
	}

	report := EvaluateLPCConformance(input)
	if report.LPCDetected {
		t.Error("LPCDetected = true, want false")
	}
	if len(report.Checks) != 0 {
		t.Errorf("Checks len = %d, want 0 when LPC not detected", len(report.Checks))
	}
	if report.Summary.Total != 0 {
		t.Errorf("Summary.Total = %d, want 0", report.Summary.Total)
	}
}

func TestEvaluateLPCConformance_LPCDetected(t *testing.T) {
	input := LPCConformanceInput{
		UseCases: []DeviceUseCases{
			{DeviceAddr: "devA", UseCases: []UseCaseInfo{
				{Abbreviation: "MPC", Available: true},
				{Abbreviation: "LPC", Available: true},
			}},
		},
	}

	report := EvaluateLPCConformance(input)
	if !report.LPCDetected {
		t.Error("LPCDetected = false, want true when LPC is present")
	}
}

// --- TS-006 EG heartbeat ---

const (
	testEGDev = "egDev"
	testCSDev = "csDev"
)

func makeHeartbeat(id int64, ts time.Time, src, dst string) *model.Message {
	return &model.Message{
		ID:           id,
		Timestamp:    ts,
		FunctionSet:  "DeviceDiagnosisHeartbeatData",
		DeviceSource: src,
		DeviceDest:   dst,
	}
}

// lpcInputWithRoles returns an input that announces EG and CS roles for LPC.
func lpcInputWithRoles(msgs []*model.Message) LPCConformanceInput {
	return LPCConformanceInput{
		Messages: msgs,
		UseCases: []DeviceUseCases{
			{DeviceAddr: testEGDev, Actor: "energyGuard", UseCases: []UseCaseInfo{
				{Abbreviation: "LPC", Available: true},
			}},
			{DeviceAddr: testCSDev, Actor: "controllableSystem", UseCases: []UseCaseInfo{
				{Abbreviation: "LPC", Available: true},
			}},
		},
	}
}

// findCheck pulls a check by requirement ID, failing the test if absent.
func findCheck(t *testing.T, report LPCConformanceReport, reqID string) LPCCheckResult {
	t.Helper()
	for _, c := range report.Checks {
		if c.RequirementID == reqID {
			return c
		}
	}
	t.Fatalf("requirement %q not found in report (have %d checks)", reqID, len(report.Checks))
	return LPCCheckResult{}
}

func TestCheckEGHeartbeat_Pass(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	msgs := []*model.Message{
		makeHeartbeat(1, now, testEGDev, testCSDev),
		makeHeartbeat(2, now.Add(30*time.Second), testEGDev, testCSDev),
		makeHeartbeat(3, now.Add(60*time.Second), testEGDev, testCSDev),
	}

	report := EvaluateLPCConformance(lpcInputWithRoles(msgs))
	check := findCheck(t, report, "LPC-TS-006")

	if check.Verdict != LPCPass {
		t.Errorf("verdict = %q, want %q (summary=%q)", check.Verdict, LPCPass, check.Summary)
	}
	if len(check.Evidence) != 0 {
		t.Errorf("evidence len = %d, want 0", len(check.Evidence))
	}
}

func TestCheckEGHeartbeat_PassExactly60s(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	msgs := []*model.Message{
		makeHeartbeat(1, now, testEGDev, testCSDev),
		makeHeartbeat(2, now.Add(60*time.Second), testEGDev, testCSDev),
	}

	report := EvaluateLPCConformance(lpcInputWithRoles(msgs))
	check := findCheck(t, report, "LPC-TS-006")

	if check.Verdict != LPCPass {
		t.Errorf("verdict = %q, want %q (60s gap is the boundary, should pass)", check.Verdict, LPCPass)
	}
}

func TestCheckEGHeartbeat_FailOnGap(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	// gap between hb#2 and hb#3 is 65s — violates 60s rule
	msgs := []*model.Message{
		makeHeartbeat(1, now, testEGDev, testCSDev),
		makeHeartbeat(2, now.Add(30*time.Second), testEGDev, testCSDev),
		makeHeartbeat(3, now.Add(95*time.Second), testEGDev, testCSDev),
	}

	report := EvaluateLPCConformance(lpcInputWithRoles(msgs))
	check := findCheck(t, report, "LPC-TS-006")

	if check.Verdict != LPCFail {
		t.Errorf("verdict = %q, want %q", check.Verdict, LPCFail)
	}
	if len(check.Evidence) == 0 {
		t.Fatal("evidence should not be empty on fail")
	}
	ev := check.Evidence[0]
	if len(ev.MessageIDs) != 2 || ev.MessageIDs[0] != 2 || ev.MessageIDs[1] != 3 {
		t.Errorf("evidence MessageIDs = %v, want [2 3]", ev.MessageIDs)
	}
	if !strings.Contains(ev.Note, "65") {
		t.Errorf("evidence note %q should mention 65s gap", ev.Note)
	}
}

func TestCheckEGHeartbeat_FailJustOver60s(t *testing.T) {
	// Strict: 60.001s gap MUST fail per locked-in decision.
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	msgs := []*model.Message{
		makeHeartbeat(1, now, testEGDev, testCSDev),
		makeHeartbeat(2, now.Add(60*time.Second+time.Millisecond), testEGDev, testCSDev),
	}

	report := EvaluateLPCConformance(lpcInputWithRoles(msgs))
	check := findCheck(t, report, "LPC-TS-006")

	if check.Verdict != LPCFail {
		t.Errorf("verdict = %q, want %q (strict 60s boundary)", check.Verdict, LPCFail)
	}
}

func TestCheckEGHeartbeat_Inconclusive(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	msgs := []*model.Message{
		makeHeartbeat(1, now, testEGDev, testCSDev),
	}

	report := EvaluateLPCConformance(lpcInputWithRoles(msgs))
	check := findCheck(t, report, "LPC-TS-006")

	if check.Verdict != LPCInconclusive {
		t.Errorf("verdict = %q, want %q (only 1 heartbeat)", check.Verdict, LPCInconclusive)
	}
}

func TestCheckEGHeartbeat_NotApplicable_NoEGRole(t *testing.T) {
	// LPC is detected but no device has actor=energyGuard.
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	msgs := []*model.Message{
		makeHeartbeat(1, now, testCSDev, testEGDev),
		makeHeartbeat(2, now.Add(30*time.Second), testCSDev, testEGDev),
	}
	input := LPCConformanceInput{
		Messages: msgs,
		UseCases: []DeviceUseCases{
			{DeviceAddr: testCSDev, Actor: "controllableSystem", UseCases: []UseCaseInfo{
				{Abbreviation: "LPC", Available: true},
			}},
		},
	}

	report := EvaluateLPCConformance(input)
	check := findCheck(t, report, "LPC-TS-006")

	if check.Verdict != LPCNotApplicable {
		t.Errorf("verdict = %q, want %q (no EG role announced)", check.Verdict, LPCNotApplicable)
	}
}

func TestCheckEGHeartbeat_IgnoresCSHeartbeats(t *testing.T) {
	// A 200s gap in CS heartbeats must not affect the EG check.
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	msgs := []*model.Message{
		makeHeartbeat(1, now, testEGDev, testCSDev),
		makeHeartbeat(2, now.Add(30*time.Second), testEGDev, testCSDev),
		makeHeartbeat(3, now, testCSDev, testEGDev),
		makeHeartbeat(4, now.Add(200*time.Second), testCSDev, testEGDev),
	}

	report := EvaluateLPCConformance(lpcInputWithRoles(msgs))
	check := findCheck(t, report, "LPC-TS-006")

	if check.Verdict != LPCPass {
		t.Errorf("verdict = %q, want %q (CS gaps must not pollute EG verdict)", check.Verdict, LPCPass)
	}
}

// --- TS-007 CS heartbeat ---

func TestCheckCSHeartbeat_Pass(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	msgs := []*model.Message{
		makeHeartbeat(1, now, testCSDev, testEGDev),
		makeHeartbeat(2, now.Add(30*time.Second), testCSDev, testEGDev),
	}

	report := EvaluateLPCConformance(lpcInputWithRoles(msgs))
	check := findCheck(t, report, "LPC-TS-007")

	if check.Verdict != LPCPass {
		t.Errorf("verdict = %q, want %q", check.Verdict, LPCPass)
	}
}

func TestCheckCSHeartbeat_FailOnGap(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	msgs := []*model.Message{
		makeHeartbeat(1, now, testCSDev, testEGDev),
		makeHeartbeat(2, now.Add(75*time.Second), testCSDev, testEGDev),
	}

	report := EvaluateLPCConformance(lpcInputWithRoles(msgs))
	check := findCheck(t, report, "LPC-TS-007")

	if check.Verdict != LPCFail {
		t.Errorf("verdict = %q, want %q", check.Verdict, LPCFail)
	}
	if !strings.Contains(check.Summary, "75") {
		t.Errorf("summary %q should mention the 75s gap", check.Summary)
	}
}

// --- TS-001 APCL >= 0 ---

func apclWriteMsg(id int64, ts time.Time, limitID int, value float64) *model.Message {
	payload := json.RawMessage(`{"datagram":{"payload":{"cmd":[{"loadControlLimitListData":{"loadControlLimitData":[` +
		`{"limitId":` + strconv.Itoa(limitID) + `,"isLimitActive":true,"value":{"number":` + strconv.FormatFloat(value, 'f', -1, 64) + `,"scale":0}}` +
		`]}}]}}}`)
	return &model.Message{
		ID:            id,
		Timestamp:     ts,
		FunctionSet:   "LoadControlLimitListData",
		CmdClassifier: "write",
		MsgCounter:    strconv.FormatInt(id, 10),
		DeviceSource:  testEGDev,
		DeviceDest:    testCSDev,
		SpinePayload:  payload,
	}
}

func TestCheckAPCLNonNegative_Pass(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	msgs := []*model.Message{
		apclWriteMsg(10, now, 1, 4200),
		apclWriteMsg(11, now.Add(time.Second), 1, 0),
	}
	report := EvaluateLPCConformance(lpcInputWithRoles(msgs))
	check := findCheck(t, report, "LPC-TS-001")
	if check.Verdict != LPCPass {
		t.Errorf("verdict = %q, want %q (summary=%q)", check.Verdict, LPCPass, check.Summary)
	}
}

func TestCheckAPCLNonNegative_FailOnNegativeValue(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	msgs := []*model.Message{
		apclWriteMsg(10, now, 1, 4200),
		apclWriteMsg(11, now.Add(time.Second), 1, -500),
	}
	report := EvaluateLPCConformance(lpcInputWithRoles(msgs))
	check := findCheck(t, report, "LPC-TS-001")
	if check.Verdict != LPCFail {
		t.Errorf("verdict = %q, want %q", check.Verdict, LPCFail)
	}
	if len(check.Evidence) != 1 {
		t.Fatalf("evidence len = %d, want 1", len(check.Evidence))
	}
	if check.Evidence[0].MessageIDs[0] != 11 {
		t.Errorf("evidence MessageID = %v, want 11", check.Evidence[0].MessageIDs)
	}
}

func TestCheckAPCLNonNegative_NA_NoWrites(t *testing.T) {
	report := EvaluateLPCConformance(lpcInputWithRoles(nil))
	check := findCheck(t, report, "LPC-TS-001")
	if check.Verdict != LPCNotApplicable {
		t.Errorf("verdict = %q, want %q (no writes)", check.Verdict, LPCNotApplicable)
	}
}

func TestCheckAPCLNonNegative_IgnoresReplies(t *testing.T) {
	// A reply with a negative value is not a violation — only writes count.
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	payload := json.RawMessage(`{"datagram":{"payload":{"cmd":[{"loadControlLimitListData":{"loadControlLimitData":[{"limitId":1,"value":{"number":-100,"scale":0}}]}}]}}}`)
	msgs := []*model.Message{
		{ID: 20, Timestamp: now, FunctionSet: "LoadControlLimitListData", CmdClassifier: "reply",
			DeviceSource: testCSDev, DeviceDest: testEGDev, SpinePayload: payload},
	}
	report := EvaluateLPCConformance(lpcInputWithRoles(msgs))
	check := findCheck(t, report, "LPC-TS-001")
	if check.Verdict != LPCNotApplicable {
		t.Errorf("verdict = %q, want %q (replies aren't writes)", check.Verdict, LPCNotApplicable)
	}
}

func TestCheckCSHeartbeat_NotApplicable_NoCSRole(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	msgs := []*model.Message{makeHeartbeat(1, now, testEGDev, testCSDev)}
	input := LPCConformanceInput{
		Messages: msgs,
		UseCases: []DeviceUseCases{
			{DeviceAddr: testEGDev, Actor: "energyGuard", UseCases: []UseCaseInfo{
				{Abbreviation: "LPC", Available: true},
			}},
		},
	}

	report := EvaluateLPCConformance(input)
	check := findCheck(t, report, "LPC-TS-007")

	if check.Verdict != LPCNotApplicable {
		t.Errorf("verdict = %q, want %q (no CS role)", check.Verdict, LPCNotApplicable)
	}
}
