package ble

import (
	"spin-hud/internal/session"
)

// selfCheck validates parsers and the session engine with known packets.
// Mirrors _self_check in spin_hud.py; grows with milestone 2.
func selfCheck() int {
	fail := 0

	check := func(name string, cond bool) {
		if !cond {
			fail++
		}
	}

	// HR: 16-bit little-endian
	bpm, ok := ParseHR([]byte{0x01, 0x60, 0x00})
	check("hr-16bit", ok && bpm == 96)

	// HR: 8-bit
	bpm, ok = ParseHR([]byte{0x00, 0x72})
	check("hr-8bit", ok && bpm == 114)

	// Crank cadence: 2 revs in 1024 ticks = 120 rpm
	data := []byte{0x02, 0x02, 0x00, 0x00, 0x04}
	rpm, ok, ref := ParseCSCCrank(data, &CSCRef{Value: 0, Event: 0})
	check("crank-120rpm", ok && rpm == 120 && ref != nil)

	// Crank counter reset (>25 revs in 1 event) yields 0, not a spike
	rpm, ok, _ = ParseCSCCrank([]byte{0x02, 0xFF, 0x00, 0x00, 0x04}, &CSCRef{Value: 0, Event: 0})
	check("crank-reset", ok && rpm == 0)

	// Wheel: 10 revs, 1024 ticks, circ 1.4363m -> 10*1.4363 m/s... compute:
	// speed_mps = 10*1.4363*1024/1024 = 14.363 m/s = 32.1 mph
	mph, mi, ref := ParseCSCWheel([]byte{0x01, 0x0A, 0x00, 0x00, 0x00, 0x00, 0x04}, &CSCRef{Value: 0, Event: 0}, 1.4363)
	check("wheel-speed", mph > 32.0 && mph < 32.2 && mi > 0.008 && mi < 0.009 && ref != nil)

	// Wheel baseline: first packet returns zero delta
	_, mi, _ = ParseCSCWheel([]byte{0x01, 0x0A, 0x00, 0x00, 0x00, 0x00, 0x04}, nil, 1.4363)
	check("wheel-baseline", mi == 0)

	// Wheel discontinuity (>100 revs) yields zeros
	mph, mi, _ = ParseCSCWheel([]byte{0x01, 0xFF, 0x00, 0x00, 0x00, 0x00, 0x04}, &CSCRef{Value: 0, Event: 0}, 1.4363)
	check("wheel-discontinuity", mph == 0 && mi == 0)

	// Session engine: calorie integration and zone mapping
	st := session.NewState(session.DefaultPlaylistID)
	hr := 150
	st.UpdateTelemetry(session.Telemetry{HR: &hr})
	z, _, _, _ := session.HRZone(&hr, 190)
	check("zone-3-at-150", z == 3)

	return fail
}
