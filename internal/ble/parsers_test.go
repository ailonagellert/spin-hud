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
	if !st.ToggleWorkoutTimer() {
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

func TestKnobMultipliesPower(t *testing.T) {
	low := session.VirtualWatts(20, session.KnobLow)
	med := session.VirtualWatts(20, session.KnobMed)
	hard := session.VirtualWatts(20, session.KnobHard)
	if low <= 0 || med <= low || hard <= med {
		t.Fatalf("low=%d med=%d hard=%d", low, med, hard)
	}
	if session.VirtualWatts(0.4, session.KnobHard) != 0 {
		t.Fatal("below 0.5 mph is 0")
	}
	st := session.NewState(session.DefaultPlaylistID)
	if st.NudgeKnob(true) != session.KnobMed {
		t.Fatal("tighten low -> med")
	}
	if st.NudgeKnob(false) != session.KnobLow {
		t.Fatal("loosen med -> low")
	}
	if st.NudgeKnob(false) != session.KnobLow {
		t.Fatal("loosen low stays")
	}
}

func TestSelfCheck(t *testing.T) {
	if SelfCheck() != 0 {
		t.Fatal("self-check failed")
	}
}

func TestParseCyclingPower(t *testing.T) {
	// Standard 250W power packet (flags=0x0000, instantaneous_power=250)
	packet := []byte{0x00, 0x00, 0xFA, 0x00}
	watts, ok := ParseCyclingPower(packet)
	if !ok || watts != 250 {
		t.Fatalf("expected 250W, got %d (ok=%v)", watts, ok)
	}

	// Clamp negative / invalid
	badPacket := []byte{0x00, 0x00, 0xFF, 0x7F} // 32767W > 3000W
	watts, ok = ParseCyclingPower(badPacket)
	if !ok || watts != 0 {
		t.Fatalf("expected 0W clamp on extreme power, got %d", watts)
	}

	// Short packet
	if _, ok := ParseCyclingPower([]byte{0x00, 0x00}); ok {
		t.Fatal("short packet should return ok=false")
	}
}

func TestParseFTMSIndoorBike(t *testing.T) {
	// Flags: Speed present (bit 0 = 0), Cadence present (bit 2 = 1 -> 0x0004), Power present (bit 6 = 1 -> 0x0040)
	// Flags = 0x0044
	// Speed = 2500 (25.00 km/h -> 15.534 mph)
	// Cadence = 180 (90.0 RPM)
	// Power = 220 Watts
	packet := []byte{
		0x44, 0x00, // Flags
		0xC4, 0x09, // Speed: 2500 (0x09C4)
		0xB4, 0x00, // Cadence: 180 (0x00B4) -> 90.0 RPM
		0xDC, 0x00, // Power: 220 (0x00DC) -> 220 Watts
	}
	data, ok := ParseFTMSIndoorBike(packet)
	if !ok {
		t.Fatal("FTMS parse failed")
	}
	if data.SpeedMPH == nil || *data.SpeedMPH < 15.5 || *data.SpeedMPH > 15.6 {
		t.Fatalf("unexpected speed: %v", data.SpeedMPH)
	}
	if data.CadenceRPM == nil || *data.CadenceRPM != 90.0 {
		t.Fatalf("unexpected cadence: %v", data.CadenceRPM)
	}
	if data.PowerWatts == nil || *data.PowerWatts != 220 {
		t.Fatalf("unexpected power: %v", data.PowerWatts)
	}
}
