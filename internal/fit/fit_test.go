package fit

import (
	"bytes"
	"testing"
	"time"
)

func TestEncodeActivity(t *testing.T) {
	now := time.Now()
	hr100 := 100
	hr150 := 150
	act := ActivityData{
		StartTime:   now.Add(-20 * time.Minute),
		EndTime:     now,
		ElapsedSec:  1200,
		DistanceM:   8500.0,
		Calories:    240,
		AvgHR:       140,
		MaxHR:       175,
		AvgCadence:  85,
		MaxCadence:  110,
		AvgSpeedMps: 7.08,
		MaxSpeedMps: 11.2,
		AvgWatts:    195,
		MaxWatts:    320,
		Trackpoints: []Trackpoint{
			{Time: now.Add(-20 * time.Minute), HR: &hr100, Cadence: 80, SpeedMps: 6.0, DistanceM: 0, Watts: 150},
			{Time: now.Add(-10 * time.Minute), HR: &hr150, Cadence: 90, SpeedMps: 8.0, DistanceM: 4000, Watts: 220},
		},
	}

	buf, err := EncodeActivity(act)
	if err != nil {
		t.Fatalf("EncodeActivity failed: %v", err)
	}

	if len(buf) < 16 {
		t.Fatalf("FIT data too small: %d bytes", len(buf))
	}

	// Verify Header
	if buf[0] != 14 {
		t.Fatalf("Header size = %d; want 14", buf[0])
	}
	if !bytes.Equal(buf[8:12], []byte(".FIT")) {
		t.Fatalf("FIT signature mismatch: %s", string(buf[8:12]))
	}

	// Verify file CRC
	dataToCheck := buf[:len(buf)-2]
	calcCRC := calculateCRC(dataToCheck)
	var gotCRC uint16
	gotCRC = uint16(buf[len(buf)-2]) | (uint16(buf[len(buf)-1]) << 8)
	if calcCRC != gotCRC {
		t.Fatalf("CRC mismatch: calculated %04X vs in-file %04X", calcCRC, gotCRC)
	}

	// Verify Record Definition (Global Msg 20) contains field 5 for distance (0x05, 0x04, 0x86)
	expectedRecordField5 := []byte{0x05, 0x04, 0x86}
	if !bytes.Contains(buf, expectedRecordField5) {
		t.Fatalf("FIT payload missing Record distance field 5 definition (0x05, 0x04, 0x86)")
	}
}
