package workout

import (
	"testing"
)

func TestParseJSON(t *testing.T) {
	jsonSample := `{
		"name": "Test Interval Session",
		"description": "5x1min test",
		"steps": [
			{"name": "Warmup", "type": "warmup", "duration_sec": 300, "cadence_target": 85},
			{"name": "Fast 1", "type": "interval", "duration_sec": 60, "cadence_target": 110, "knob": "hard"}
		]
	}`

	w, err := ParseJSON([]byte(jsonSample))
	if err != nil {
		t.Fatalf("ParseJSON failed: %v", err)
	}
	if w.Name != "Test Interval Session" {
		t.Fatalf("unexpected name: %s", w.Name)
	}
	if len(w.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(w.Steps))
	}
	if w.TotalDurationSec != 360 {
		t.Fatalf("expected 360s total, got %d", w.TotalDurationSec)
	}
}

func TestParseZWO(t *testing.T) {
	zwoSample := `<?xml version="1.0" encoding="UTF-8"?>
<workout_file>
    <author>Zwift Coach</author>
    <name>Zwift Sweet Spot</name>
    <description>30 minutes sweetspot endurance</description>
    <sportType>bike</sportType>
    <workout>
        <Warmup Duration="300" PowerLow="0.40" PowerHigh="0.75" Cadence="85"/>
        <SteadyState Duration="600" Power="0.90" Cadence="90"/>
        <IntervalsT Repeat="3" OnDuration="60" OffDuration="60" OnPower="1.15" OffPower="0.50" Cadence="100" CadenceResting="80"/>
        <Cooldown Duration="300" PowerLow="0.75" PowerHigh="0.40" Cadence="80"/>
    </workout>
</workout_file>`

	w, err := ParseZWO([]byte(zwoSample))
	if err != nil {
		t.Fatalf("ParseZWO failed: %v", err)
	}
	if w.Name != "Zwift Sweet Spot" {
		t.Fatalf("unexpected name: %s", w.Name)
	}
	// Steps: 1 warmup + 1 steady + 3*2 intervals + 1 cooldown = 9 steps
	if len(w.Steps) != 9 {
		t.Fatalf("expected 9 steps, got %d", len(w.Steps))
	}
	// Total duration: 300 + 600 + 3*(60+60) + 300 = 1560s
	if w.TotalDurationSec != 1560 {
		t.Fatalf("expected 1560s total, got %d", w.TotalDurationSec)
	}
}

func TestParseERGMRC(t *testing.T) {
	mrcSample := `[COURSE HEADER]
VERSION = 2
UNITS = ENGLISH
DESCRIPTION = 10-Min Micro Burst
FILE NAME = microburst.mrc
MINUTES PERCENT
[END COURSE HEADER]
[COURSE DATA]
0.00 50
2.00 50
2.00 120
3.00 120
3.00 60
5.00 60
[END COURSE DATA]`

	w, err := ParseERGMRC([]byte(mrcSample))
	if err != nil {
		t.Fatalf("ParseERGMRC failed: %v", err)
	}
	if len(w.Steps) < 2 {
		t.Fatalf("expected at least 2 steps, got %d", len(w.Steps))
	}
	if w.TotalDurationSec <= 0 {
		t.Fatalf("expected positive total duration, got %d", w.TotalDurationSec)
	}
}

func TestDetectAndParse(t *testing.T) {
	jsonSample := `{"name":"Quick Spin","steps":[{"name":"Spin","duration_sec":600}]}`
	w, err := DetectAndParse("workout.json", []byte(jsonSample))
	if err != nil || w.Name != "Quick Spin" {
		t.Fatalf("DetectAndParse JSON failed: %v", err)
	}
}
