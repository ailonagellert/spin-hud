package ble

import (
	"testing"

	"spin-hud/internal/session"
)

func TestParseHR(t *testing.T) {
	if bpm, ok := ParseHR([]byte{0x01, 0x60, 0x00}); !ok || bpm != 96 {
		t.Fatalf("16-bit HR = %d, %v", bpm, ok)
	}
	if bpm, ok := ParseHR([]byte{0x00, 0x72}); !ok || bpm != 114 {
		t.Fatalf("8-bit HR = %d, %v", bpm, ok)
	}
	if _, ok := ParseHR(nil); ok {
		t.Fatal("empty HR packet should not parse")
	}
}

func TestParseCSCCrank(t *testing.T) {
	// 2 revs in 2048 ticks = 60 rpm
	rpm, ok, ref := ParseCSCCrank([]byte{0x02, 0x02, 0x00, 0x00, 0x08}, &CSCRef{Value: 0, Event: 0})
	if !ok || rpm != 60 || ref == nil {
		t.Fatalf("crank = %f, %v", rpm, ok)
	}

	// Baseline: first packet establishes reference only
	_, ok, ref = ParseCSCCrank([]byte{0x02, 0x02, 0x00, 0x00, 0x08}, nil)
	if ok || ref == nil {
		t.Fatal("first crank packet must be baseline-only")
	}

	// Counter reset / discontinuity (>25 revs in 1 event) -> 0, not a spike
	rpm, ok, _ = ParseCSCCrank([]byte{0x02, 0xFF, 0x00, 0x00, 0x04}, &CSCRef{Value: 0, Event: 0})
	if !ok || rpm != 0 {
		t.Fatalf("crank reset = %f, %v; want 0", rpm, ok)
	}

	// Plausibility clamp (>250 rpm -> 0)
	rpm, ok, _ = ParseCSCCrank([]byte{0x02, 0x10, 0x00, 0x00, 0x01}, &CSCRef{Value: 0, Event: 0})
	if !ok || rpm != 0 {
		t.Fatalf("crank clamp = %f, %v; want 0", rpm, ok)
	}
}

func TestParseCSCWheel(t *testing.T) {
	const circ = 1.4363

	// Baseline: first packet returns zero delta (reconnect boundary trap)
	mph, mi, ref := ParseCSCWheel([]byte{0x01, 0x0A, 0x00, 0x00, 0x00, 0x00, 0x04}, nil, circ)
	if mph != 0 || mi != 0 || ref == nil {
		t.Fatalf("wheel baseline = %f mph, %f mi", mph, mi)
	}

	// 10 revs in 1024 ticks: 14.363 m/s = 32.11 mph
	mph, mi, _ = ParseCSCWheel([]byte{0x01, 0x0A, 0x00, 0x00, 0x00, 0x00, 0x04}, &CSCRef{Value: 0, Event: 0}, circ)
	if mph < 32.0 || mph > 32.2 {
		t.Fatalf("wheel speed = %f; want ~32.11", mph)
	}
	wantMi := 10 * circ / 1609.344
	if mi < wantMi*0.99 || mi > wantMi*1.01 {
		t.Fatalf("wheel delta = %f; want ~%f", mi, wantMi)
	}

	// Discontinuity: >100 revs in 1 event -> zeros
	mph, mi, _ = ParseCSCWheel([]byte{0x01, 0xFF, 0x00, 0x00, 0x00, 0x00, 0x04}, &CSCRef{Value: 0, Event: 0}, circ)
	if mph != 0 || mi != 0 {
		t.Fatalf("wheel discontinuity = %f mph, %f mi", mph, mi)
	}

	// Unrealistic speed (>120 mph) -> zeros
	mph, mi, _ = ParseCSCWheel([]byte{0x01, 0xFF, 0x00, 0x00, 0x00, 0x01, 0x00}, &CSCRef{Value: 0, Event: 0}, circ)
	if mph != 0 || mi != 0 {
		t.Fatalf("wheel overspeed = %f mph, %f mi", mph, mi)
	}
}

func TestHRZone(t *testing.T) {
	rest := 0
	if z, _, _, _ := session.HRZone(&rest, 190); z != 0 {
		t.Fatalf("resting zone = %d", z)
	}
	h := 100 // 0.526 -> zone 1
	if z, _, _, _ := session.HRZone(&h, 190); z != 1 {
		t.Fatalf("zone at 100 = %d", z)
	}
	h = 150 // 0.789 -> zone 3
	if z, _, _, _ := session.HRZone(&h, 190); z != 3 {
		t.Fatalf("zone at 150 = %d", z)
	}
	h = 185 // 0.97 -> zone 5
	if z, _, _, _ := session.HRZone(&h, 190); z != 5 {
		t.Fatalf("zone at 185 = %d", z)
	}
}

func TestToggleAndCalories(t *testing.T) {
	st := session.NewState(session.DefaultPlaylistID)
	if !st.GetSnapshot().IsRunning {
		t.Fatal("workout starts running (Python parity)")
	}
	// Calories: hr 150 for ~1s adds (150-55)*0.0022*1 = 0.209 kcal
	hr := 150
	st.UpdateTelemetry(session.Telemetry{HR: &hr})
	c1 := st.GetSnapshot().Calories
	if c1 != 0 { // dt=0 on first update (lastCalTime == now) -> no calories yet
		t.Fatalf("calories after first HR = %d; want 0", c1)
	}
	if st.ToggleWorkoutTimer() {
		t.Fatal("toggle should pause")
	}
	if st.ToggleWorkoutTimer() {
		t.Fatal("second toggle should resume")
	}
	st.ResetWorkout()
	if !st.GetSnapshot().IsRunning {
		t.Fatal("reset restarts workout running (Python parity)")
	}
	if st.GetSnapshot().Calories != 0 {
		t.Fatal("reset clears calories")
	}
}

func TestVirtualPower(t *testing.T) {
	st := session.NewState(session.DefaultPlaylistID)
	spd := 20.0 // mph
	st.UpdateTelemetry(session.Telemetry{SpeedMPH: &spd})
	snap := st.GetSnapshot()
	v := 20 * 0.44704 // 8.9408 m/s
	want := int(3.5*v + 0.35*v*v*v)
	if snap.Watts != want {
		t.Fatalf("watts = %d; want %d", snap.Watts, want)
	}
	if snap.AvgWatts == nil || *snap.AvgWatts != want {
		t.Fatalf("avg watts mismatch")
	}
}

func TestSelfCheck(t *testing.T) {
	if SelfCheck() != 0 {
		t.Fatal("self-check failed")
	}
}
