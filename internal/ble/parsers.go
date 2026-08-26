package ble

import (
	"encoding/binary"
)

// CSCRef is the previous (cumulative value, event time) pair used to compute deltas.
type CSCRef struct {
	Value uint32
	Event uint16
}

// ParseHR parses a BLE Heart Rate Measurement (0x2A37) and returns BPM.
func ParseHR(data []byte) (int, bool) {
	if len(data) == 0 {
		return 0, false
	}
	flags := data[0]
	if flags&0x01 != 0 {
		if len(data) < 3 {
			return 0, false
		}
		return int(binary.LittleEndian.Uint16(data[1:3])), true
	}
	if len(data) < 2 {
		return 0, false
	}
	return int(data[1]), true
}


// ParseCSCCrank parses crank cadence (RPM) from a CSC measurement (0x2A5B).
// Returns (rpm, ok, newRef). A counter reset/discontinuity or implausible
// reading yields 0.0 with ok=true so the UI shows zero rather than a spike.
func ParseCSCCrank(data []byte, prev *CSCRef) (rpm float64, ok bool, now *CSCRef) {
	if len(data) == 0 {
		return 0, false, prev
	}
	flags := data[0]
	off := 1
	if flags&0x01 != 0 {
		off += 6 // skip wheel data
	}
	if flags&0x02 == 0 || len(data) < off+4 {
		return 0, false, prev
	}
	revs := binary.LittleEndian.Uint16(data[off : off+2])
	ev := binary.LittleEndian.Uint16(data[off+2 : off+4])
	ref := &CSCRef{Value: uint32(revs), Event: ev}
	if prev == nil {
		return 0, false, ref
	}
	dRevs := (uint32(revs) - prev.Value) & 0xFFFF
	dTicks := (ev - prev.Event) & 0xFFFF
	if dTicks == 0 {
		return 0, false, ref
	}
	if dRevs > 25 { // Counter reset / discontinuity
		return 0.0, true, ref
	}
	r := float64(dRevs) * 1024 * 60 / float64(dTicks)
	if r > 250.0 { // Plausibility clamp
		return 0.0, true, ref
	}
	return r, true, ref
}

// ParseCSCWheel parses wheel speed (mph) and delta distance (miles) from a
// CSC measurement (0x2A5B). Returns (speedMPH, deltaMiles, newRef).
// A speed of 0 is returned (not None) when wheels are present but stationary.
func ParseCSCWheel(data []byte, prev *CSCRef, wheelCircM float64) (speedMPH float64, deltaMiles float64, now *CSCRef) {
	if len(data) == 0 {
		return 0, 0, prev
	}
	flags := data[0]
	if flags&0x01 == 0 || len(data) < 7 {
		return 0, 0, prev
	}
	revs := binary.LittleEndian.Uint32(data[1:5])
	ev := binary.LittleEndian.Uint16(data[5:7])
	ref := &CSCRef{Value: revs, Event: ev}
	if prev == nil {
		// First packet after (re)connect: establish baseline only, delta = 0.
		return 0, 0, ref
	}
	dRevs := (revs - prev.Value) & 0xFFFFFFFF
	dTicks := (ev - prev.Event) & 0xFFFF

	// Discontinuity / counter reboot / unrealistic jump (e.g. > 100 revs in 1 event)
	if dRevs > 100 || dTicks == 0 {
		return 0, 0, ref
	}

	if dRevs == 0 {
		return 0, 0, ref
	}

	deltaM := float64(dRevs) * wheelCircM
	deltaMi := deltaM / 1609.344

	speedMps := float64(dRevs) * wheelCircM * 1024.0 / float64(dTicks)
	mph := speedMps * 2.236936

	// Unrealistic speed (> 120 mph is a corrupted packet/reboot)
	if mph > 120.0 {
		return 0, 0, ref
	}

	return mph, deltaMi, ref
}
