package session

import (
	"math"
	"time"
)

// Telemetry is the input bundle for UpdateTelemetry (mirrors Python kwargs).
// Only non-nil fields are applied.
type Telemetry struct {
	HR            *int
	Cadence       *float64
	SpeedMPH      *float64
	DistanceMiles *float64
	Status        *string
}

// UpdateTelemetry applies new sensor values and accumulates statistics
// only for the fields that were updated (mirrors update_telemetry).
func (s *State) UpdateTelemetry(t Telemetry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()

	// Heart Rate update
	if t.HR != nil {
		newHR := *t.HR
		s.hr = &newHR
		if s.isRunning && newHR > 0 {
			s.hrSum += float64(newHR)
			s.hrCount++
			if newHR > s.maxHRVal {
				s.maxHRVal = newHR
			}
			// Calorie integration: forward only, bounded to sane dt windows
			dt := now.Sub(s.lastCalTime).Seconds()
			if dt > 0 && dt < 10.0 {
				if newHR > 75 {
					s.calories += (float64(newHR) - 55) * 0.0022 * dt
				}
			}
			s.lastCalTime = now
		}
	}

	// Cadence update
	if t.Cadence != nil {
		newCad := *t.Cadence
		s.cadence = &newCad
		if s.isRunning && newCad > 0 {
			s.cadSum += newCad
			s.cadCount++
			if newCad > s.maxCad {
				s.maxCad = newCad
			}
		}
	}

	// Speed update & virtual power accumulation
	if t.SpeedMPH != nil {
		newSpd := *t.SpeedMPH
		s.speedMPH = &newSpd
		if s.isRunning && newSpd > 0.5 {
			s.spdSum += newSpd
			s.spdCount++
			if newSpd > s.maxSpdMPH {
				s.maxSpdMPH = newSpd
			}
			v := newSpd * 0.44704
			w := int(math.Round(3.5*v + 0.35*math.Pow(v, 3)))
			s.wattsSum += float64(w)
			s.wattsCount++
			if w > s.maxWatts {
				s.maxWatts = w
			}
		}
	}

	// Distance override (if specified directly)
	if t.DistanceMiles != nil && s.isRunning {
		s.distanceMiles = *t.DistanceMiles
	}

	if t.Status != nil {
		s.status = *t.Status
	}
}

// AddDistanceDelta adds delta distance accumulated from wheel revolutions.
func (s *State) AddDistanceDelta(deltaMiles float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isRunning && deltaMiles > 0 {
		s.distanceMiles += deltaMiles
	}
}
