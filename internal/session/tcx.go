package session

import (
	"fmt"
	"strings"
	"time"
)

// ApplySettings updates settings after /api/settings validation.
func (s *State) ApplySettings(playlistID string, wheelCircM float64, maxHR int, riderWeightKg float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PlaylistID = playlistID
	s.WheelCircM = wheelCircM
	s.MaxHR = maxHR
	s.RiderWeightKg = riderWeightKg
}

// WorkoutStartWall returns the wall-clock start of the current workout.
func (s *State) WorkoutStartWall() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.workoutStartWall
}

const tcxTimeLayout = "2006-01-02T15:04:05Z"

// GenerateTCX produces a Training Center XML activity file for Strava/Garmin
// Connect, mirroring generate_tcx in spin_hud.py.
func GenerateTCX(s *State) string {
	snap := s.GetSnapshot()

	s.mu.RLock()
	startTimeISO := s.workoutStartWall.UTC().Format(tcxTimeLayout)
	totalTime := snap.ElapsedSec
	totalDistM := round1(s.distanceMiles * 1609.344)
	calories := int(round1(s.calories))
	avgHR := 0
	if snap.AvgHR != nil {
		avgHR = *snap.AvgHR
	}
	maxHR := s.maxHRVal
	maxSpeedMps := 0.0
	if s.maxSpdMPH > 0 {
		maxSpeedMps = round2(s.maxSpdMPH * 0.44704)
	}
	avgCad := 0
	if snap.AvgCadence != nil {
		avgCad = *snap.AvgCadence
	}
	avgWatts := 0
	if snap.AvgWatts != nil {
		avgWatts = *snap.AvgWatts
	}
	points := make([]Trackpoint, len(s.trackpoints))
	copy(points, s.trackpoints)
	startWall := s.workoutStartWall
	s.mu.RUnlock()

	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<TrainingCenterDatabase\n")
	b.WriteString("  xmlns=\"http://www.garmin.com/xmlschemas/TrainingCenterDatabase/v2\"\n")
	b.WriteString("  xmlns:ns2=\"http://www.garmin.com/xmlschemas/ActivityExtension/v2\"\n")
	b.WriteString("  xmlns:xsi=\"http://www.w3.org/2001/XMLSchema-instance\"\n")
	b.WriteString("  xsi:schemaLocation=\"http://www.garmin.com/xmlschemas/TrainingCenterDatabase/v2 http://www.garmin.com/xmlschemas/TrainingCenterDatabasev2.xsd\">\n")
	b.WriteString("  <Activities>\n")
	b.WriteString("    <Activity Sport=\"Biking\">\n")
	fmt.Fprintf(&b, "      <Id>%s</Id>\n", startTimeISO)
	fmt.Fprintf(&b, "      <Lap StartTime=\"%s\">\n", startTimeISO)
	fmt.Fprintf(&b, "        <TotalTimeSeconds>%d</TotalTimeSeconds>\n", totalTime)
	fmt.Fprintf(&b, "        <DistanceMeters>%s</DistanceMeters>\n", num(totalDistM))
	fmt.Fprintf(&b, "        <MaximumSpeed>%s</MaximumSpeed>\n", num(maxSpeedMps))
	fmt.Fprintf(&b, "        <Calories>%d</Calories>\n", calories)
	if avgHR > 0 {
		b.WriteString("        <AverageHeartRateBpm>\n")
		fmt.Fprintf(&b, "          <Value>%d</Value>\n", avgHR)
		b.WriteString("        </AverageHeartRateBpm>\n")
	}
	if maxHR > 0 {
		b.WriteString("        <MaximumHeartRateBpm>\n")
		fmt.Fprintf(&b, "          <Value>%d</Value>\n", maxHR)
		b.WriteString("        </MaximumHeartRateBpm>\n")
	}
	b.WriteString("        <Intensity>Active</Intensity>\n")
	fmt.Fprintf(&b, "        <Cadence>%d</Cadence>\n", avgCad)
	b.WriteString("        <TriggerMethod>Manual</TriggerMethod>\n")
	b.WriteString("        <Track>\n")

	if len(points) == 0 {
		var hrPtr *int
		if avgHR > 0 {
			hrPtr = &avgHR
		}
		points = []Trackpoint{{
			Time:     startWall,
			HR:       hrPtr,
			Cadence:  avgCad,
			DistM:    totalDistM,
			SpeedMps: 0.0,
			Watts:    avgWatts,
		}}
	}

	for _, pt := range points {
		ptISO := pt.Time.UTC().Format(tcxTimeLayout)
		b.WriteString("          <Trackpoint>\n")
		fmt.Fprintf(&b, "            <Time>%s</Time>\n", ptISO)
		fmt.Fprintf(&b, "            <DistanceMeters>%s</DistanceMeters>\n", num(pt.DistM))
		if pt.HR != nil && *pt.HR > 0 {
			b.WriteString("            <HeartRateBpm>\n")
			fmt.Fprintf(&b, "              <Value>%d</Value>\n", *pt.HR)
			b.WriteString("            </HeartRateBpm>\n")
		}
		if pt.Cadence > 0 {
			fmt.Fprintf(&b, "            <Cadence>%d</Cadence>\n", pt.Cadence)
		}
		b.WriteString("            <Extensions>\n")
		b.WriteString("              <ns2:TPX>\n")
		fmt.Fprintf(&b, "                <ns2:Speed>%s</ns2:Speed>\n", num(pt.SpeedMps))
		fmt.Fprintf(&b, "                <ns2:Watts>%d</ns2:Watts>\n", pt.Watts)
		b.WriteString("              </ns2:TPX>\n")
		b.WriteString("            </Extensions>\n")
		b.WriteString("          </Trackpoint>\n")
	}

	b.WriteString("        </Track>\n")
	b.WriteString("      </Lap>\n")
	b.WriteString("      <Creator xsi:type=\"Device_t\">\n")
	b.WriteString("        <Name>Spin Studio HUD</Name>\n")
	b.WriteString("        <UnitId>5881436</UnitId>\n")
	b.WriteString("        <ProductID>1</ProductID>\n")
	b.WriteString("        <Version>\n")
	b.WriteString("          <VersionMajor>1</VersionMajor>\n")
	b.WriteString("          <VersionMinor>0</VersionMinor>\n")
	b.WriteString("        </Version>\n")
	b.WriteString("      </Creator>\n")
	b.WriteString("    </Activity>\n")
	b.WriteString("  </Activities>\n")
	b.WriteString("</TrainingCenterDatabase>")
	return b.String()
}

// num formats a float the way Python str(float) does (at least one decimal,
// no trailing zeros, no exponent).
func num(v float64) string {
	s := strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.6f", v), "0"), ".")
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}
