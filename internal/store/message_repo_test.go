package store

import (
	"fmt"
	"testing"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
)

func createTestTrace(t *testing.T, db *DB) *model.Trace {
	t.Helper()
	repo := NewTraceRepo(db)
	trace := &model.Trace{
		Name:      "Test",
		StartedAt: time.Now().Truncate(time.Second),
		CreatedAt: time.Now().Truncate(time.Second),
	}
	if err := repo.CreateTrace(trace); err != nil {
		t.Fatalf("CreateTrace failed: %v", err)
	}
	return trace
}

func TestMessageRepo_InsertAndGet(t *testing.T) {
	db := newTestDB(t)
	trace := createTestTrace(t, db)
	repo := NewMessageRepo(db)

	msg := &model.Message{
		TraceID:       trace.ID,
		SequenceNum:   1,
		Timestamp:     time.Now().Truncate(time.Second),
		Direction:     model.DirectionIncoming,
		ShipMsgType:   model.ShipMsgTypeData,
		CmdClassifier: "read",
		FunctionSet:   "MeasurementListData",
	}
	if err := repo.InsertMessage(msg); err != nil {
		t.Fatalf("InsertMessage failed: %v", err)
	}
	if msg.ID == 0 {
		t.Error("expected ID to be set")
	}

	got, err := repo.GetMessage(trace.ID, msg.ID)
	if err != nil {
		t.Fatalf("GetMessage failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetMessage returned nil")
	}
	if got.CmdClassifier != "read" {
		t.Errorf("CmdClassifier = %q, want %q", got.CmdClassifier, "read")
	}
	if got.FunctionSet != "MeasurementListData" {
		t.Errorf("FunctionSet = %q, want %q", got.FunctionSet, "MeasurementListData")
	}
}

func TestMessageRepo_BatchInsertAndPaginate(t *testing.T) {
	db := newTestDB(t)
	trace := createTestTrace(t, db)
	repo := NewMessageRepo(db)

	msgs := make([]*model.Message, 100)
	for i := range msgs {
		msgs[i] = &model.Message{
			TraceID:     trace.ID,
			SequenceNum: i + 1,
			Timestamp:   time.Now().Truncate(time.Second),
			ShipMsgType: model.ShipMsgTypeData,
			FunctionSet: fmt.Sprintf("func_%d", i%5),
		}
	}
	if err := repo.InsertMessages(msgs); err != nil {
		t.Fatalf("InsertMessages failed: %v", err)
	}

	count, err := repo.CountMessages(trace.ID)
	if err != nil {
		t.Fatalf("CountMessages failed: %v", err)
	}
	if count != 100 {
		t.Errorf("count = %d, want 100", count)
	}

	// Paginate: first page
	page1, err := repo.ListMessages(trace.ID, MessageFilter{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("ListMessages page 1 failed: %v", err)
	}
	if len(page1) != 10 {
		t.Errorf("page 1 len = %d, want 10", len(page1))
	}
	if page1[0].SequenceNum != 1 {
		t.Errorf("first item seq = %d, want 1", page1[0].SequenceNum)
	}

	// Paginate: second page
	page2, err := repo.ListMessages(trace.ID, MessageFilter{Limit: 10, Offset: 10})
	if err != nil {
		t.Fatalf("ListMessages page 2 failed: %v", err)
	}
	if len(page2) != 10 {
		t.Errorf("page 2 len = %d, want 10", len(page2))
	}
	if page2[0].SequenceNum != 11 {
		t.Errorf("second page first seq = %d, want 11", page2[0].SequenceNum)
	}
}

func TestMessageRepo_FilterByCmdClassifier(t *testing.T) {
	db := newTestDB(t)
	trace := createTestTrace(t, db)
	repo := NewMessageRepo(db)

	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: time.Now(), ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read"},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: time.Now(), ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply"},
		{TraceID: trace.ID, SequenceNum: 3, Timestamp: time.Now(), ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read"},
	}
	if err := repo.InsertMessages(msgs); err != nil {
		t.Fatalf("InsertMessages failed: %v", err)
	}

	reads, err := repo.ListMessages(trace.ID, MessageFilter{CmdClassifier: "read"})
	if err != nil {
		t.Fatalf("ListMessages failed: %v", err)
	}
	if len(reads) != 2 {
		t.Errorf("filtered len = %d, want 2", len(reads))
	}
}

func TestFtsPrefix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"LoadContro", "LoadContro*"},
		{"Measurement", "Measurement*"},
		{"LoadControlLimitListData", "LoadControlLimitListData*"},
		{"load control", "load* control*"},
		{"already*", "already*"},
		{"  spaced  ", "spaced*"},
		{"LoadControl OR Setpoint", "LoadControl* OR Setpoint*"},
		{"LoadControl AND Setpoint", "LoadControl* AND Setpoint*"},
		{"Measurement NOT Heartbeat", "Measurement* NOT Heartbeat*"},
		{"load or set", "load* OR set*"},
	}
	for _, tt := range tests {
		got := ftsPrefix(tt.input)
		if got != tt.want {
			t.Errorf("ftsPrefix(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMessageRepo_FTSSearch(t *testing.T) {
	db := newTestDB(t)
	trace := createTestTrace(t, db)
	repo := NewMessageRepo(db)

	now := time.Now().Truncate(time.Second)
	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, SpinePayload: []byte(`{"MeasurementListData":"values"}`)},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, SpinePayload: []byte(`{"DeviceClassificationData":"info"}`)},
		{TraceID: trace.ID, SequenceNum: 3, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, SpinePayload: []byte(`{"MeasurementListData":"more_values"}`)},
	}
	if err := repo.InsertMessages(msgs); err != nil {
		t.Fatalf("InsertMessages failed: %v", err)
	}

	results, err := repo.ListMessages(trace.ID, MessageFilter{Search: "MeasurementListData"})
	if err != nil {
		t.Fatalf("FTS search failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("FTS search len = %d, want 2", len(results))
	}
}

func TestMessageRepo_FTSPartialSearch(t *testing.T) {
	db := newTestDB(t)
	trace := createTestTrace(t, db)
	repo := NewMessageRepo(db)

	now := time.Now().Truncate(time.Second)
	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, SpinePayload: []byte(`{"LoadControlLimitListData":"x"}`)},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, SpinePayload: []byte(`{"LoadControlLimitDescriptionListData":"y"}`)},
		{TraceID: trace.ID, SequenceNum: 3, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, SpinePayload: []byte(`{"MeasurementListData":"z"}`)},
	}
	if err := repo.InsertMessages(msgs); err != nil {
		t.Fatalf("InsertMessages failed: %v", err)
	}

	// Partial search "LoadContro" should match both LoadControl messages
	results, err := repo.ListMessages(trace.ID, MessageFilter{Search: "LoadContro"})
	if err != nil {
		t.Fatalf("FTS partial search failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("FTS partial search len = %d, want 2", len(results))
	}
}

func TestMessageRepo_FTSBooleanSearch(t *testing.T) {
	db := newTestDB(t)
	trace := createTestTrace(t, db)
	repo := NewMessageRepo(db)

	now := time.Now().Truncate(time.Second)
	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, SpinePayload: []byte(`{"LoadControlLimitListData":"x"}`)},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, SpinePayload: []byte(`{"SetpointListData":"y"}`)},
		{TraceID: trace.ID, SequenceNum: 3, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, SpinePayload: []byte(`{"MeasurementListData":"z"}`)},
	}
	if err := repo.InsertMessages(msgs); err != nil {
		t.Fatalf("InsertMessages failed: %v", err)
	}

	// OR search should match LoadControl and Setpoint but not Measurement
	results, err := repo.ListMessages(trace.ID, MessageFilter{Search: "LoadControl OR Setpoint"})
	if err != nil {
		t.Fatalf("FTS OR search failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("FTS OR search len = %d, want 2", len(results))
	}

	// Lowercase "or" should also work
	results, err = repo.ListMessages(trace.ID, MessageFilter{Search: "LoadControl or Setpoint"})
	if err != nil {
		t.Fatalf("FTS lowercase or search failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("FTS lowercase or search len = %d, want 2", len(results))
	}
}

func TestMessageRepo_FilterByTimeRange(t *testing.T) {
	db := newTestDB(t)
	trace := createTestTrace(t, db)
	repo := NewMessageRepo(db)

	base := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: base, ShipMsgType: model.ShipMsgTypeData},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: base.Add(5 * time.Minute), ShipMsgType: model.ShipMsgTypeData},
		{TraceID: trace.ID, SequenceNum: 3, Timestamp: base.Add(10 * time.Minute), ShipMsgType: model.ShipMsgTypeData},
		{TraceID: trace.ID, SequenceNum: 4, Timestamp: base.Add(15 * time.Minute), ShipMsgType: model.ShipMsgTypeData},
	}
	if err := repo.InsertMessages(msgs); err != nil {
		t.Fatalf("InsertMessages failed: %v", err)
	}

	from := base.Add(3 * time.Minute)
	to := base.Add(12 * time.Minute)
	results, err := repo.ListMessages(trace.ID, MessageFilter{TimeFrom: &from, TimeTo: &to})
	if err != nil {
		t.Fatalf("time range filter failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("time range filter len = %d, want 2", len(results))
	}
}

func TestMessageRepo_FilterByDevice(t *testing.T) {
	db := newTestDB(t)
	trace := createTestTrace(t, db)
	repo := NewMessageRepo(db)

	now := time.Now().Truncate(time.Second)
	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, DeviceSource: "deviceA", DeviceDest: "deviceB"},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, DeviceSource: "deviceB", DeviceDest: "deviceC"},
		{TraceID: trace.ID, SequenceNum: 3, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, DeviceSource: "deviceC", DeviceDest: "deviceA"},
	}
	if err := repo.InsertMessages(msgs); err != nil {
		t.Fatalf("InsertMessages failed: %v", err)
	}

	// Filter by Device (source OR dest)
	results, err := repo.ListMessages(trace.ID, MessageFilter{Device: "deviceA"})
	if err != nil {
		t.Fatalf("device filter failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("device filter len = %d, want 2", len(results))
	}

	// Filter by DeviceSource only
	results, err = repo.ListMessages(trace.ID, MessageFilter{DeviceSource: "deviceB"})
	if err != nil {
		t.Fatalf("device source filter failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("device source filter len = %d, want 1", len(results))
	}
}

func TestMessageRepo_CombinedFilters(t *testing.T) {
	db := newTestDB(t)
	trace := createTestTrace(t, db)
	repo := NewMessageRepo(db)

	now := time.Now().Truncate(time.Second)
	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read", DeviceSource: "devA", SpinePayload: []byte(`{"MeasurementListData":"x"}`)},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply", DeviceSource: "devA", SpinePayload: []byte(`{"MeasurementListData":"y"}`)},
		{TraceID: trace.ID, SequenceNum: 3, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read", DeviceSource: "devB", SpinePayload: []byte(`{"DeviceClassification":"z"}`)},
	}
	if err := repo.InsertMessages(msgs); err != nil {
		t.Fatalf("InsertMessages failed: %v", err)
	}

	// Combine FTS + cmdClassifier + device
	results, err := repo.ListMessages(trace.ID, MessageFilter{
		Search:        "MeasurementListData",
		CmdClassifier: "read",
		DeviceSource:  "devA",
	})
	if err != nil {
		t.Fatalf("combined filter failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("combined filter len = %d, want 1", len(results))
	}
}

func TestMessageRepo_EmptySearchReturnsAll(t *testing.T) {
	db := newTestDB(t)
	trace := createTestTrace(t, db)
	repo := NewMessageRepo(db)

	now := time.Now().Truncate(time.Second)
	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: now, ShipMsgType: model.ShipMsgTypeData},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: now, ShipMsgType: model.ShipMsgTypeData},
	}
	if err := repo.InsertMessages(msgs); err != nil {
		t.Fatalf("InsertMessages failed: %v", err)
	}

	results, err := repo.ListMessages(trace.ID, MessageFilter{})
	if err != nil {
		t.Fatalf("empty filter failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("empty filter len = %d, want 2", len(results))
	}
}

func TestMessageRepo_FindByMsgCounter(t *testing.T) {
	db := newTestDB(t)
	trace := createTestTrace(t, db)
	repo := NewMessageRepo(db)

	now := time.Now().Truncate(time.Second)
	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, MsgCounter: "42"},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, MsgCounter: "43", MsgCounterRef: "42"},
		{TraceID: trace.ID, SequenceNum: 3, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, MsgCounter: "44"},
	}
	if err := repo.InsertMessages(msgs); err != nil {
		t.Fatalf("InsertMessages failed: %v", err)
	}

	byCounter, err := repo.FindByMsgCounter(trace.ID, "42")
	if err != nil {
		t.Fatalf("FindByMsgCounter failed: %v", err)
	}
	if len(byCounter) != 1 {
		t.Errorf("FindByMsgCounter len = %d, want 1", len(byCounter))
	}

	byRef, err := repo.FindByMsgCounterRef(trace.ID, "42")
	if err != nil {
		t.Fatalf("FindByMsgCounterRef failed: %v", err)
	}
	if len(byRef) != 1 {
		t.Errorf("FindByMsgCounterRef len = %d, want 1", len(byRef))
	}
	if byRef[0].MsgCounter != "43" {
		t.Errorf("FindByMsgCounterRef[0].MsgCounter = %q, want %q", byRef[0].MsgCounter, "43")
	}
}

func TestMessageRepo_CountFilteredMessages(t *testing.T) {
	db := newTestDB(t)
	trace := createTestTrace(t, db)
	repo := NewMessageRepo(db)

	now := time.Now().Truncate(time.Second)
	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read", FunctionSet: "MeasurementListData", SpinePayload: []byte(`{"MeasurementListData":"x"}`)},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply", FunctionSet: "MeasurementListData", SpinePayload: []byte(`{"MeasurementListData":"y"}`)},
		{TraceID: trace.ID, SequenceNum: 3, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read", FunctionSet: "LoadControlLimitListData", SpinePayload: []byte(`{"LoadControlLimitListData":"z"}`)},
		{TraceID: trace.ID, SequenceNum: 4, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "write", FunctionSet: "LoadControlLimitListData", SpinePayload: []byte(`{"LoadControlLimitListData":"w"}`)},
		{TraceID: trace.ID, SequenceNum: 5, Timestamp: now, ShipMsgType: "init"},
	}
	if err := repo.InsertMessages(msgs); err != nil {
		t.Fatalf("InsertMessages failed: %v", err)
	}

	tests := []struct {
		name   string
		filter MessageFilter
		want   int
	}{
		{"no filter", MessageFilter{}, 5},
		{"by classifier", MessageFilter{CmdClassifier: "read"}, 2},
		{"by FTS search", MessageFilter{Search: "MeasurementListData"}, 2},
		{"by classifier and FTS", MessageFilter{CmdClassifier: "read", Search: "MeasurementListData"}, 1},
		{"limit/offset ignored", MessageFilter{Limit: 1, Offset: 2}, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, err := repo.CountFilteredMessages(trace.ID, tt.filter)
			if err != nil {
				t.Fatalf("CountFilteredMessages failed: %v", err)
			}
			if count != tt.want {
				t.Errorf("count = %d, want %d", count, tt.want)
			}
		})
	}
}

func TestMessageRepo_FindConversationMessages(t *testing.T) {
	db := newTestDB(t)
	trace := createTestTrace(t, db)
	repo := NewMessageRepo(db)

	now := time.Now().Truncate(time.Second)
	msgs := []*model.Message{
		// Conversation: devA <-> devB, MeasurementListData
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read", FunctionSet: "MeasurementListData", DeviceSource: "devA", DeviceDest: "devB"},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: now.Add(time.Second), ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply", FunctionSet: "MeasurementListData", DeviceSource: "devB", DeviceDest: "devA"},
		{TraceID: trace.ID, SequenceNum: 3, Timestamp: now.Add(2 * time.Second), ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "notify", FunctionSet: "MeasurementListData", DeviceSource: "devB", DeviceDest: "devA"},
		// Different function set — excluded
		{TraceID: trace.ID, SequenceNum: 4, Timestamp: now.Add(3 * time.Second), ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read", FunctionSet: "LoadControlLimitListData", DeviceSource: "devA", DeviceDest: "devB"},
		// Different device pair — excluded
		{TraceID: trace.ID, SequenceNum: 5, Timestamp: now.Add(4 * time.Second), ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read", FunctionSet: "MeasurementListData", DeviceSource: "devC", DeviceDest: "devD"},
		// SHIP handshake — excluded
		{TraceID: trace.ID, SequenceNum: 6, Timestamp: now.Add(5 * time.Second), ShipMsgType: "init", CmdClassifier: "", FunctionSet: "MeasurementListData", DeviceSource: "devA", DeviceDest: "devB"},
	}
	if err := repo.InsertMessages(msgs); err != nil {
		t.Fatalf("InsertMessages failed: %v", err)
	}

	t.Run("bidirectional", func(t *testing.T) {
		got, total, err := repo.FindConversationMessages(trace.ID, "devA", "devB", "MeasurementListData", 50, 0)
		if err != nil {
			t.Fatalf("FindConversationMessages failed: %v", err)
		}
		if total != 3 {
			t.Errorf("total = %d, want 3", total)
		}
		if len(got) != 3 {
			t.Errorf("len = %d, want 3", len(got))
		}
		// Verify ordering
		if len(got) >= 3 {
			if got[0].SequenceNum != 1 || got[1].SequenceNum != 2 || got[2].SequenceNum != 3 {
				t.Errorf("ordering: got seqs %d,%d,%d, want 1,2,3", got[0].SequenceNum, got[1].SequenceNum, got[2].SequenceNum)
			}
		}
	})

	t.Run("bidirectional reversed args", func(t *testing.T) {
		// Passing devB, devA should give the same result
		got, total, err := repo.FindConversationMessages(trace.ID, "devB", "devA", "MeasurementListData", 50, 0)
		if err != nil {
			t.Fatalf("FindConversationMessages failed: %v", err)
		}
		if total != 3 {
			t.Errorf("total = %d, want 3", total)
		}
		if len(got) != 3 {
			t.Errorf("len = %d, want 3", len(got))
		}
	})

	t.Run("different function set excluded", func(t *testing.T) {
		got, total, err := repo.FindConversationMessages(trace.ID, "devA", "devB", "LoadControlLimitListData", 50, 0)
		if err != nil {
			t.Fatalf("FindConversationMessages failed: %v", err)
		}
		if total != 1 {
			t.Errorf("total = %d, want 1", total)
		}
		if len(got) != 1 {
			t.Errorf("len = %d, want 1", len(got))
		}
	})

	t.Run("different device pair excluded", func(t *testing.T) {
		got, total, err := repo.FindConversationMessages(trace.ID, "devC", "devD", "MeasurementListData", 50, 0)
		if err != nil {
			t.Fatalf("FindConversationMessages failed: %v", err)
		}
		if total != 1 {
			t.Errorf("total = %d, want 1", total)
		}
		if len(got) != 1 {
			t.Errorf("len = %d, want 1", len(got))
		}
	})

	t.Run("pagination", func(t *testing.T) {
		got, total, err := repo.FindConversationMessages(trace.ID, "devA", "devB", "MeasurementListData", 2, 0)
		if err != nil {
			t.Fatalf("FindConversationMessages failed: %v", err)
		}
		if total != 3 {
			t.Errorf("total = %d, want 3", total)
		}
		if len(got) != 2 {
			t.Errorf("len = %d, want 2", len(got))
		}

		// Second page
		got2, total2, err := repo.FindConversationMessages(trace.ID, "devA", "devB", "MeasurementListData", 2, 2)
		if err != nil {
			t.Fatalf("FindConversationMessages page 2 failed: %v", err)
		}
		if total2 != 3 {
			t.Errorf("total = %d, want 3", total2)
		}
		if len(got2) != 1 {
			t.Errorf("len = %d, want 1", len(got2))
		}
	})
}

func TestMessageRepo_ListDistinctFunctionSets(t *testing.T) {
	db := newTestDB(t)
	trace := createTestTrace(t, db)
	repo := NewMessageRepo(db)

	now := time.Now().Truncate(time.Second)
	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, FunctionSet: "MeasurementListData"},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, FunctionSet: "MeasurementListData"},
		{TraceID: trace.ID, SequenceNum: 3, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, FunctionSet: "LoadControlLimitListData"},
		{TraceID: trace.ID, SequenceNum: 4, Timestamp: now, ShipMsgType: "init", FunctionSet: ""},           // not data type
		{TraceID: trace.ID, SequenceNum: 5, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, FunctionSet: ""}, // empty function set
	}
	if err := repo.InsertMessages(msgs); err != nil {
		t.Fatalf("InsertMessages failed: %v", err)
	}

	fsets, err := repo.ListDistinctFunctionSets(trace.ID)
	if err != nil {
		t.Fatalf("ListDistinctFunctionSets failed: %v", err)
	}
	if len(fsets) != 2 {
		t.Fatalf("expected 2 distinct function sets, got %d: %v", len(fsets), fsets)
	}
	// Sorted alphabetically
	if fsets[0] != "LoadControlLimitListData" {
		t.Errorf("fsets[0] = %q, want LoadControlLimitListData", fsets[0])
	}
	if fsets[1] != "MeasurementListData" {
		t.Errorf("fsets[1] = %q, want MeasurementListData", fsets[1])
	}

	// Empty trace
	trace2 := &model.Trace{Name: "Empty", StartedAt: now, CreatedAt: now}
	NewTraceRepo(db).CreateTrace(trace2)
	fsets2, err := repo.ListDistinctFunctionSets(trace2.ID)
	if err != nil {
		t.Fatalf("ListDistinctFunctionSets empty failed: %v", err)
	}
	if len(fsets2) != 0 {
		t.Errorf("expected 0 function sets for empty trace, got %d", len(fsets2))
	}
}

func TestMessageRepo_FindOrphanedRequestIDs(t *testing.T) {
	tests := []struct {
		name     string
		msgs     func(traceID int64, now time.Time) []*model.Message
		wantCount int
	}{
		{
			name: "mixed orphaned and answered",
			msgs: func(traceID int64, now time.Time) []*model.Message {
				return []*model.Message{
					// read with counter=10, msg2 references it → NOT orphaned
					{TraceID: traceID, SequenceNum: 1, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read", MsgCounter: "10"},
					// reply referencing 10 → excluded (reply never expects response)
					{TraceID: traceID, SequenceNum: 2, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply", MsgCounter: "11", MsgCounterRef: "10"},
					// write with counter=20, nobody references it → orphaned
					{TraceID: traceID, SequenceNum: 3, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "write", MsgCounter: "20"},
					// non-data message → excluded
					{TraceID: traceID, SequenceNum: 4, Timestamp: now, ShipMsgType: "init", CmdClassifier: "read", MsgCounter: "30"},
					// call with counter=40, nobody references it → orphaned
					{TraceID: traceID, SequenceNum: 5, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "call", MsgCounter: "40"},
				}
			},
			wantCount: 2,
		},
		{
			name: "all answered",
			msgs: func(traceID int64, now time.Time) []*model.Message {
				return []*model.Message{
					{TraceID: traceID, SequenceNum: 1, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read", MsgCounter: "10"},
					{TraceID: traceID, SequenceNum: 2, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply", MsgCounter: "11", MsgCounterRef: "10"},
				}
			},
			wantCount: 0,
		},
		{
			name: "notify and reply excluded",
			msgs: func(traceID int64, now time.Time) []*model.Message {
				return []*model.Message{
					// notify with unreferenced counter → should NOT be orphaned
					{TraceID: traceID, SequenceNum: 1, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "notify", MsgCounter: "50"},
					// reply with unreferenced counter → should NOT be orphaned
					{TraceID: traceID, SequenceNum: 2, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply", MsgCounter: "51"},
					// read with unreferenced counter → IS orphaned
					{TraceID: traceID, SequenceNum: 3, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read", MsgCounter: "52"},
				}
			},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newTestDB(t)
			trace := createTestTrace(t, db)
			repo := NewMessageRepo(db)

			now := time.Now().Truncate(time.Second)
			if err := repo.InsertMessages(tt.msgs(trace.ID, now)); err != nil {
				t.Fatalf("InsertMessages failed: %v", err)
			}

			ids, err := repo.FindOrphanedRequestIDs(trace.ID)
			if err != nil {
				t.Fatalf("FindOrphanedRequestIDs failed: %v", err)
			}

			if len(ids) != tt.wantCount {
				t.Errorf("expected %d orphaned IDs, got %d: %v", tt.wantCount, len(ids), ids)
			}
		})
	}
}

func TestMessageRepo_ListMessageSummaries(t *testing.T) {
	db := newTestDB(t)
	trace := createTestTrace(t, db)
	repo := NewMessageRepo(db)

	now := time.Now().Truncate(time.Second)
	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read", FunctionSet: "MeasurementListData", DeviceSource: "devA", DeviceDest: "devB", SpinePayload: []byte(`{"MeasurementListData":"x"}`)},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply", FunctionSet: "MeasurementListData", DeviceSource: "devB", DeviceDest: "devA", SpinePayload: []byte(`{"MeasurementListData":"y"}`)},
		{TraceID: trace.ID, SequenceNum: 3, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read", FunctionSet: "LoadControlLimitListData", DeviceSource: "devA", DeviceDest: "devB", SpinePayload: []byte(`{"LoadControlLimitListData":"z"}`)},
		{TraceID: trace.ID, SequenceNum: 4, Timestamp: now, ShipMsgType: "init"},
	}
	if err := repo.InsertMessages(msgs); err != nil {
		t.Fatalf("InsertMessages failed: %v", err)
	}

	tests := []struct {
		name   string
		filter MessageFilter
		want   int
	}{
		{"no filter returns all", MessageFilter{}, 4},
		{"filter by cmdClassifier", MessageFilter{CmdClassifier: "read"}, 2},
		{"filter by FTS search", MessageFilter{Search: "MeasurementListData"}, 2},
		{"combined filters", MessageFilter{CmdClassifier: "read", Search: "MeasurementListData"}, 1},
		{"limit/offset ignored", MessageFilter{Limit: 1, Offset: 2}, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summaries, err := repo.ListMessageSummaries(trace.ID, tt.filter)
			if err != nil {
				t.Fatalf("ListMessageSummaries failed: %v", err)
			}
			if len(summaries) != tt.want {
				t.Errorf("got %d summaries, want %d", len(summaries), tt.want)
			}
		})
	}

	// Verify summary fields are populated
	t.Run("fields populated", func(t *testing.T) {
		summaries, err := repo.ListMessageSummaries(trace.ID, MessageFilter{CmdClassifier: "read", FunctionSet: "MeasurementListData"})
		if err != nil {
			t.Fatalf("ListMessageSummaries failed: %v", err)
		}
		if len(summaries) != 1 {
			t.Fatalf("expected 1 summary, got %d", len(summaries))
		}
		s := summaries[0]
		if s.ID == 0 {
			t.Error("expected ID to be set")
		}
		if s.TraceID != trace.ID {
			t.Errorf("TraceID = %d, want %d", s.TraceID, trace.ID)
		}
		if s.SequenceNum != 1 {
			t.Errorf("SequenceNum = %d, want 1", s.SequenceNum)
		}
		if s.CmdClassifier != "read" {
			t.Errorf("CmdClassifier = %q, want %q", s.CmdClassifier, "read")
		}
		if s.FunctionSet != "MeasurementListData" {
			t.Errorf("FunctionSet = %q, want %q", s.FunctionSet, "MeasurementListData")
		}
		if s.DeviceSource != "devA" {
			t.Errorf("DeviceSource = %q, want %q", s.DeviceSource, "devA")
		}
	})

	// Verify ordering by sequence_num
	t.Run("ordered by sequence_num", func(t *testing.T) {
		summaries, err := repo.ListMessageSummaries(trace.ID, MessageFilter{})
		if err != nil {
			t.Fatalf("ListMessageSummaries failed: %v", err)
		}
		for i := 1; i < len(summaries); i++ {
			if summaries[i].SequenceNum < summaries[i-1].SequenceNum {
				t.Errorf("not ordered: seq %d before %d", summaries[i-1].SequenceNum, summaries[i].SequenceNum)
			}
		}
	})
}

func TestMessageRepo_MultiFunctionSetFilter(t *testing.T) {
	db := newTestDB(t)
	trace := createTestTrace(t, db)
	repo := NewMessageRepo(db)

	now := time.Now().Truncate(time.Second)
	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, FunctionSet: "MeasurementListData"},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, FunctionSet: "LoadControlLimitListData"},
		{TraceID: trace.ID, SequenceNum: 3, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, FunctionSet: "DeviceDiagnosisHeartbeatData"},
	}
	if err := repo.InsertMessages(msgs); err != nil {
		t.Fatalf("InsertMessages failed: %v", err)
	}

	// Single function set
	results, err := repo.ListMessages(trace.ID, MessageFilter{FunctionSet: "MeasurementListData"})
	if err != nil {
		t.Fatalf("single functionSet filter failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("single functionSet filter len = %d, want 1", len(results))
	}

	// Multiple function sets (comma-separated)
	results, err = repo.ListMessages(trace.ID, MessageFilter{FunctionSet: "MeasurementListData,LoadControlLimitListData"})
	if err != nil {
		t.Fatalf("multi functionSet filter failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("multi functionSet filter len = %d, want 2", len(results))
	}
}
