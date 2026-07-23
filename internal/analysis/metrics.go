package analysis

import (
	"math"
	"strconv"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
)

// HeartbeatJitter contains statistics about heartbeat interval jitter.
type HeartbeatJitter struct {
	DevicePair  string  `json:"devicePair"`
	MeanMs      float64 `json:"meanIntervalMs"`
	StdDevMs    float64 `json:"stdDevMs"`
	MinMs       float64 `json:"minIntervalMs"`
	MaxMs       float64 `json:"maxIntervalMs"`
	SampleCount int     `json:"sampleCount"`
}

// HeartbeatMetrics contains computed heartbeat accuracy metrics.
type HeartbeatMetrics struct {
	HeartbeatJitter []HeartbeatJitter `json:"heartbeatJitter"`
}

// ComputeHeartbeatMetrics computes heartbeat accuracy metrics from messages.
func ComputeHeartbeatMetrics(msgs []*model.Message) HeartbeatMetrics {
	result := HeartbeatMetrics{
		HeartbeatJitter: computeHeartbeatJitter(msgs),
	}

	if result.HeartbeatJitter == nil {
		result.HeartbeatJitter = []HeartbeatJitter{}
	}

	return result
}

// FormatHeartbeatCSV formats heartbeat jitter data as CSV for export.
func FormatHeartbeatCSV(jitters []HeartbeatJitter) string {
	csv := "devicePair,meanIntervalMs,stdDevMs,minIntervalMs,maxIntervalMs,sampleCount\n"
	for _, j := range jitters {
		csv += j.DevicePair + "," +
			strconv.FormatFloat(j.MeanMs, 'f', 1, 64) + "," +
			strconv.FormatFloat(j.StdDevMs, 'f', 1, 64) + "," +
			strconv.FormatFloat(j.MinMs, 'f', 1, 64) + "," +
			strconv.FormatFloat(j.MaxMs, 'f', 1, 64) + "," +
			strconv.Itoa(j.SampleCount) + "\n"
	}
	return csv
}

// --- device pair helpers ---

type pairKey struct {
	a, b string
}

func directionalPairKey(src, dst string) pairKey {
	return pairKey{src, dst}
}

func pairKeyStr(k pairKey) string {
	return k.a + " → " + k.b
}

func devicePairFromMsg(msg *model.Message) (pairKey, bool) {
	src, dst := msg.DeviceSource, msg.DeviceDest
	if src == "" && dst == "" {
		src, dst = msg.SourceAddr, msg.DestAddr
	}
	if src == "" || dst == "" {
		return pairKey{}, false
	}
	return directionalPairKey(src, dst), true
}

// --- heartbeat computation ---

// heartbeatGap is one interval between consecutive heartbeats on a device pair.
// Carries the bracketing message IDs so callers can cite specific evidence.
type heartbeatGap struct {
	FromID, ToID int64
	From, To     time.Time
	Duration     time.Duration
}

// collectHeartbeatGapsByPair walks the message stream and returns the gaps
// between consecutive heartbeats per directional device pair, in insertion
// order. Pairs with fewer than 2 heartbeats are omitted.
func collectHeartbeatGapsByPair(msgs []*model.Message) (order []pairKey, gaps map[pairKey][]heartbeatGap) {
	type point struct {
		ts time.Time
		id int64
	}
	points := map[pairKey][]point{}
	order = []pairKey{}

	for _, msg := range msgs {
		if msg.FunctionSet != "DeviceDiagnosisHeartbeatData" {
			continue
		}
		pair, ok := devicePairFromMsg(msg)
		if !ok {
			continue
		}
		if _, exists := points[pair]; !exists {
			order = append(order, pair)
		}
		points[pair] = append(points[pair], point{ts: msg.Timestamp, id: msg.ID})
	}

	gaps = map[pairKey][]heartbeatGap{}
	for _, pair := range order {
		pts := points[pair]
		if len(pts) < 2 {
			continue
		}
		for i := 1; i < len(pts); i++ {
			gaps[pair] = append(gaps[pair], heartbeatGap{
				FromID:   pts[i-1].id,
				ToID:     pts[i].id,
				From:     pts[i-1].ts,
				To:       pts[i].ts,
				Duration: pts[i].ts.Sub(pts[i-1].ts),
			})
		}
	}
	return order, gaps
}

func computeHeartbeatJitter(msgs []*model.Message) []HeartbeatJitter {
	order, gaps := collectHeartbeatGapsByPair(msgs)

	var results []HeartbeatJitter
	for _, pair := range order {
		pairGaps, ok := gaps[pair]
		if !ok || len(pairGaps) == 0 {
			continue
		}

		jitter := HeartbeatJitter{
			DevicePair:  pairKeyStr(pair),
			SampleCount: len(pairGaps),
			MinMs:       float64(pairGaps[0].Duration.Milliseconds()),
			MaxMs:       float64(pairGaps[0].Duration.Milliseconds()),
		}

		sum := 0.0
		for _, g := range pairGaps {
			iv := float64(g.Duration.Milliseconds())
			sum += iv
			if iv < jitter.MinMs {
				jitter.MinMs = iv
			}
			if iv > jitter.MaxMs {
				jitter.MaxMs = iv
			}
		}
		jitter.MeanMs = sum / float64(len(pairGaps))

		sumSq := 0.0
		for _, g := range pairGaps {
			diff := float64(g.Duration.Milliseconds()) - jitter.MeanMs
			sumSq += diff * diff
		}
		jitter.StdDevMs = math.Sqrt(sumSq / float64(len(pairGaps)))

		results = append(results, jitter)
	}
	return results
}
