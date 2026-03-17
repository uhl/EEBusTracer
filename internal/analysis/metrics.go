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

func computeHeartbeatJitter(msgs []*model.Message) []HeartbeatJitter {
	// Collect heartbeat timestamps per pair
	heartbeats := map[pairKey][]time.Time{}
	pairOrder := []pairKey{}

	for _, msg := range msgs {
		if msg.FunctionSet != "DeviceDiagnosisHeartbeatData" {
			continue
		}
		pair, ok := devicePairFromMsg(msg)
		if !ok {
			continue
		}
		if _, exists := heartbeats[pair]; !exists {
			pairOrder = append(pairOrder, pair)
		}
		heartbeats[pair] = append(heartbeats[pair], msg.Timestamp)
	}

	var results []HeartbeatJitter
	for _, pair := range pairOrder {
		timestamps := heartbeats[pair]
		if len(timestamps) < 2 {
			continue
		}

		intervals := make([]float64, len(timestamps)-1)
		for i := 1; i < len(timestamps); i++ {
			intervals[i-1] = float64(timestamps[i].Sub(timestamps[i-1]).Milliseconds())
		}

		jitter := HeartbeatJitter{
			DevicePair:  pairKeyStr(pair),
			SampleCount: len(intervals),
			MinMs:       intervals[0],
			MaxMs:       intervals[0],
		}

		sum := 0.0
		for _, iv := range intervals {
			sum += iv
			if iv < jitter.MinMs {
				jitter.MinMs = iv
			}
			if iv > jitter.MaxMs {
				jitter.MaxMs = iv
			}
		}
		jitter.MeanMs = sum / float64(len(intervals))

		// Standard deviation
		sumSq := 0.0
		for _, iv := range intervals {
			diff := iv - jitter.MeanMs
			sumSq += diff * diff
		}
		jitter.StdDevMs = math.Sqrt(sumSq / float64(len(intervals)))

		results = append(results, jitter)
	}
	return results
}
