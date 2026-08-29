package fit

import (
	"bytes"
	"encoding/binary"
	"time"
)

// fitEpoch is the base time for Garmin FIT timestamps: 1989-12-31 00:00:00 UTC.
var fitEpoch = time.Date(1989, 12, 31, 0, 0, 0, 0, time.UTC)

func toFitTimestamp(t time.Time) uint32 {
	if t.Before(fitEpoch) {
		return 0
	}
	return uint32(t.UTC().Sub(fitEpoch).Seconds())
}

var crcTable = [16]uint16{
	0x0000, 0xCC01, 0xD801, 0x1400, 0xF001, 0x3C00, 0x2800, 0xE401,
	0xA001, 0x6C00, 0x7800, 0xB401, 0x5000, 0x9C01, 0x8801, 0x4400,
}

func updateCRC(crc uint16, b byte) uint16 {
	tmp := crcTable[crc&0xF]
	crc = (crc >> 4) ^ tmp ^ crcTable[b&0xF]
	tmp = crcTable[crc&0xF]
	crc = (crc >> 4) ^ tmp ^ crcTable[(b>>4)&0xF]
	return crc
}

func calculateCRC(data []byte) uint16 {
	var crc uint16
	for _, b := range data {
		crc = updateCRC(crc, b)
	}
	return crc
}

// ActivityData holds the complete workout dataset to encode into FIT.
type ActivityData struct {
	StartTime   time.Time
	EndTime     time.Time
	ElapsedSec  int
	DistanceM   float64
	Calories    int
	AvgHR       int
	MaxHR       int
	AvgCadence  int
	MaxCadence  int
	AvgSpeedMps float64
	MaxSpeedMps float64
	AvgWatts    int
	MaxWatts    int
	Trackpoints []Trackpoint
}

type Trackpoint struct {
	Time      time.Time
	HR        *int
	Cadence   int
	SpeedMps  float64
	DistanceM float64
	Watts     int
}

// EncodeActivity encodes an ActivityData into binary FIT format.
func EncodeActivity(data ActivityData) ([]byte, error) {
	var recBuf bytes.Buffer

	startTS := toFitTimestamp(data.StartTime)
	endTS := toFitTimestamp(data.EndTime)
	if endTS == 0 {
		endTS = startTS + uint32(data.ElapsedSec)
	}

	// 1. FileId Definition & Data (Global Msg 0, Local Msg 0)
	fileIdDef := []byte{
		0x40, 0x00, 0x00, 0x00, 0x00, 0x05,
		0x00, 0x01, 0x00, // type: enum, 1 byte
		0x01, 0x02, 0x84, // manufacturer: uint16, 2 bytes
		0x02, 0x02, 0x84, // product: uint16, 2 bytes
		0x03, 0x04, 0x8C, // serial_number: uint32z, 4 bytes
		0x04, 0x04, 0x86, // time_created: uint32, 4 bytes
	}
	recBuf.Write(fileIdDef)
	recBuf.WriteByte(0x00) // Data header (Local Msg 0)
	recBuf.WriteByte(4)    // type = Activity
	binary.Write(&recBuf, binary.LittleEndian, uint16(255))
	binary.Write(&recBuf, binary.LittleEndian, uint16(1))
	binary.Write(&recBuf, binary.LittleEndian, uint32(1001))
	binary.Write(&recBuf, binary.LittleEndian, startTS)

	// 2. Event Start Definition & Data (Global Msg 21, Local Msg 1)
	eventDef := []byte{
		0x41, 0x00, 0x00, 0x15, 0x00, 0x03,
		0xFD, 0x04, 0x86, // timestamp
		0x00, 0x01, 0x00, // event
		0x01, 0x01, 0x00, // event_type
	}
	recBuf.Write(eventDef)
	recBuf.WriteByte(0x01) // Data header (Local Msg 1)
	binary.Write(&recBuf, binary.LittleEndian, startTS)
	recBuf.WriteByte(0) // Timer
	recBuf.WriteByte(0) // Start

	// 3. Record Definition & Data (Global Msg 20, Local Msg 2)
	recordDef := []byte{
		0x42, 0x00, 0x00, 0x14, 0x00, 0x06,
		0xFD, 0x04, 0x86, // timestamp
		0x00, 0x04, 0x86, // distance (cm)
		0x06, 0x02, 0x84, // speed (mm/s)
		0x07, 0x02, 0x84, // power (watts)
		0x03, 0x01, 0x02, // heart_rate (bpm)
		0x04, 0x01, 0x02, // cadence (rpm)
	}
	recBuf.Write(recordDef)

	if len(data.Trackpoints) > 0 {
		for _, pt := range data.Trackpoints {
			ptTS := toFitTimestamp(pt.Time)
			distCM := uint32(pt.DistanceM * 100.0)
			speedMMS := uint16(pt.SpeedMps * 1000.0)
			powerW := uint16(pt.Watts)
			hrBpm := uint8(0)
			if pt.HR != nil && *pt.HR > 0 {
				hrBpm = uint8(*pt.HR)
			}
			cadRpm := uint8(pt.Cadence)

			recBuf.WriteByte(0x02) // Data header (Local Msg 2)
			binary.Write(&recBuf, binary.LittleEndian, ptTS)
			binary.Write(&recBuf, binary.LittleEndian, distCM)
			binary.Write(&recBuf, binary.LittleEndian, speedMMS)
			binary.Write(&recBuf, binary.LittleEndian, powerW)
			recBuf.WriteByte(hrBpm)
			recBuf.WriteByte(cadRpm)
		}
	} else {
		distCM := uint32(data.DistanceM * 100.0)
		speedMMS := uint16(data.AvgSpeedMps * 1000.0)
		powerW := uint16(data.AvgWatts)
		hrBpm := uint8(data.AvgHR)
		cadRpm := uint8(data.AvgCadence)

		recBuf.WriteByte(0x02)
		binary.Write(&recBuf, binary.LittleEndian, startTS)
		binary.Write(&recBuf, binary.LittleEndian, distCM)
		binary.Write(&recBuf, binary.LittleEndian, speedMMS)
		binary.Write(&recBuf, binary.LittleEndian, powerW)
		recBuf.WriteByte(hrBpm)
		recBuf.WriteByte(cadRpm)
	}

	// 4. Event Stop Definition & Data (Local Msg 1)
	recBuf.WriteByte(0x01) // Data header (Local Msg 1)
	binary.Write(&recBuf, binary.LittleEndian, endTS)
	recBuf.WriteByte(0) // Timer
	recBuf.WriteByte(4) // StopAll

	// 5. Lap Definition & Data (Global Msg 19, Local Msg 3)
	lapDef := []byte{
		0x43, 0x00, 0x00, 0x13, 0x00, 0x0E,
		0xFD, 0x04, 0x86, // timestamp
		0x02, 0x04, 0x86, // start_time
		0x07, 0x04, 0x86, // total_elapsed_time
		0x08, 0x04, 0x86, // total_timer_time
		0x09, 0x04, 0x86, // total_distance
		0x0B, 0x02, 0x84, // total_calories
		0x0D, 0x02, 0x84, // avg_speed
		0x0E, 0x02, 0x84, // max_speed
		0x0F, 0x01, 0x02, // avg_heart_rate
		0x10, 0x01, 0x02, // max_heart_rate
		0x11, 0x01, 0x02, // avg_cadence
		0x12, 0x01, 0x02, // max_cadence
		0x13, 0x02, 0x84, // avg_power
		0x14, 0x02, 0x84, // max_power
	}
	recBuf.Write(lapDef)
	recBuf.WriteByte(0x03) // Data header (Local Msg 3)
	binary.Write(&recBuf, binary.LittleEndian, endTS)
	binary.Write(&recBuf, binary.LittleEndian, startTS)
	binary.Write(&recBuf, binary.LittleEndian, uint32(data.ElapsedSec*1000))
	binary.Write(&recBuf, binary.LittleEndian, uint32(data.ElapsedSec*1000))
	binary.Write(&recBuf, binary.LittleEndian, uint32(data.DistanceM*100.0))
	binary.Write(&recBuf, binary.LittleEndian, uint16(data.Calories))
	binary.Write(&recBuf, binary.LittleEndian, uint16(data.AvgSpeedMps*1000.0))
	binary.Write(&recBuf, binary.LittleEndian, uint16(data.MaxSpeedMps*1000.0))
	recBuf.WriteByte(uint8(data.AvgHR))
	recBuf.WriteByte(uint8(data.MaxHR))
	recBuf.WriteByte(uint8(data.AvgCadence))
	recBuf.WriteByte(uint8(data.MaxCadence))
	binary.Write(&recBuf, binary.LittleEndian, uint16(data.AvgWatts))
	binary.Write(&recBuf, binary.LittleEndian, uint16(data.MaxWatts))

	// 6. Session Definition & Data (Global Msg 18, Local Msg 4)
	sessionDef := []byte{
		0x44, 0x00, 0x00, 0x12, 0x00, 0x10,
		0xFD, 0x04, 0x86, // timestamp
		0x02, 0x04, 0x86, // start_time
		0x05, 0x01, 0x00, // sport
		0x06, 0x01, 0x00, // sub_sport
		0x07, 0x04, 0x86, // total_elapsed_time
		0x08, 0x04, 0x86, // total_timer_time
		0x09, 0x04, 0x86, // total_distance
		0x0B, 0x02, 0x84, // total_calories
		0x0E, 0x02, 0x84, // avg_speed
		0x0F, 0x02, 0x84, // max_speed
		0x10, 0x01, 0x02, // avg_heart_rate
		0x11, 0x01, 0x02, // max_heart_rate
		0x12, 0x01, 0x02, // avg_cadence
		0x13, 0x01, 0x02, // max_cadence
		0x14, 0x02, 0x84, // avg_power
		0x15, 0x02, 0x84, // max_power
	}
	recBuf.Write(sessionDef)
	recBuf.WriteByte(0x04) // Data header (Local Msg 4)
	binary.Write(&recBuf, binary.LittleEndian, endTS)
	binary.Write(&recBuf, binary.LittleEndian, startTS)
	recBuf.WriteByte(2) // Sport: Cycling
	recBuf.WriteByte(6) // SubSport: Indoor Cycling
	binary.Write(&recBuf, binary.LittleEndian, uint32(data.ElapsedSec*1000))
	binary.Write(&recBuf, binary.LittleEndian, uint32(data.ElapsedSec*1000))
	binary.Write(&recBuf, binary.LittleEndian, uint32(data.DistanceM*100.0))
	binary.Write(&recBuf, binary.LittleEndian, uint16(data.Calories))
	binary.Write(&recBuf, binary.LittleEndian, uint16(data.AvgSpeedMps*1000.0))
	binary.Write(&recBuf, binary.LittleEndian, uint16(data.MaxSpeedMps*1000.0))
	recBuf.WriteByte(uint8(data.AvgHR))
	recBuf.WriteByte(uint8(data.MaxHR))
	recBuf.WriteByte(uint8(data.AvgCadence))
	recBuf.WriteByte(uint8(data.MaxCadence))
	binary.Write(&recBuf, binary.LittleEndian, uint16(data.AvgWatts))
	binary.Write(&recBuf, binary.LittleEndian, uint16(data.MaxWatts))

	recordsBytes := recBuf.Bytes()
	dataSize := uint32(len(recordsBytes))

	// 14-byte Header
	header := make([]byte, 14)
	header[0] = 14
	header[1] = 0x20
	binary.LittleEndian.PutUint16(header[2:4], 2140)
	binary.LittleEndian.PutUint32(header[4:8], dataSize)
	copy(header[8:12], []byte(".FIT"))
	headerCRC := calculateCRC(header[:12])
	binary.LittleEndian.PutUint16(header[12:14], headerCRC)

	fullData := append(header, recordsBytes...)
	fileCRC := calculateCRC(fullData)
	crcBytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(crcBytes, fileCRC)

	return append(fullData, crcBytes...), nil
}
