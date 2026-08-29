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

// ParseCyclingPower parses instantaneous power (Watts) from BLE Cycling Power Measurement (0x2A63).
func ParseCyclingPower(data []byte) (int, bool) {
	if len(data) < 4 {
		return 0, false
	}
	rawPower := int16(binary.LittleEndian.Uint16(data[2:4]))
	if rawPower < 0 {
		return 0, true
	}
	if rawPower > 3000 { // Plausibility clamp (> 3000 W)
		return 0, true
	}
	return int(rawPower), true
}

// FTMSData contains parsed metrics from FTMS Indoor Bike Data (0x2AD2).
type FTMSData struct {
	SpeedMPH   *float64
	CadenceRPM *float64
	PowerWatts *int
	HeartRate  *int
}

// ParseFTMSIndoorBike parses metrics from BLE FTMS Indoor Bike Data (0x2AD2).
func ParseFTMSIndoorBike(data []byte) (FTMSData, bool) {
	if len(data) < 2 {
		return FTMSData{}, false
	}
	flags := binary.LittleEndian.Uint16(data[0:2])
	offset := 2
	var res FTMSData

	// Bit 0: More Data (0 = Instantaneous Speed present)
	if flags&0x0001 == 0 {
		if len(data) < offset+2 {
			return res, false
		}
		rawSpd := binary.LittleEndian.Uint16(data[offset : offset+2])
		offset += 2
		spdKmh := float64(rawSpd) * 0.01
		spdMph := spdKmh / 1.609344
		if spdMph <= 120.0 {
			res.SpeedMPH = &spdMph
		}
	}

	// Bit 1: Average Speed Present
	if flags&0x0002 != 0 {
		offset += 2
	}

	// Bit 2: Instantaneous Cadence Present (0.5 RPM)
	if flags&0x0004 != 0 {
		if len(data) < offset+2 {
			return res, false
		}
		rawCad := binary.LittleEndian.Uint16(data[offset : offset+2])
		offset += 2
		cadRpm := float64(rawCad) * 0.5
		if cadRpm <= 250.0 {
			res.CadenceRPM = &cadRpm
		}
	}

	// Bit 3: Average Cadence Present
	if flags&0x0008 != 0 {
		offset += 2
	}

	// Bit 4: Total Distance Present (uint24)
	if flags&0x0010 != 0 {
		offset += 3
	}

	// Bit 5: Resistance Level Present
	if flags&0x0020 != 0 {
		offset += 2
	}

	// Bit 6: Instantaneous Power Present (sint16 in Watts)
	if flags&0x0040 != 0 {
		if len(data) < offset+2 {
			return res, false
		}
		rawWatts := int16(binary.LittleEndian.Uint16(data[offset : offset+2]))
		offset += 2
		w := int(rawWatts)
		if w < 0 {
			w = 0
		} else if w > 3000 {
			w = 0
		}
		res.PowerWatts = &w
	}

	// Bit 7: Average Power Present
	if flags&0x0080 != 0 {
		offset += 2
	}

	// Bit 8: Expended Energy Present (5 bytes)
	if flags&0x0100 != 0 {
		offset += 5
	}

	// Bit 9: Heart Rate Present (uint8)
	if flags&0x0200 != 0 {
		if len(data) >= offset+1 {
			hr := int(data[offset])
			offset += 1
			if hr > 0 && hr <= 250 {
				res.HeartRate = &hr
			}
		}
	}

	return res, true
}

