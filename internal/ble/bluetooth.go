package ble

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"tinygo.org/x/bluetooth"

	"spin-hud/internal/session"
)

var (
	hrServiceUUID    = bluetooth.ServiceUUIDHeartRate
	hrCharUUID       = bluetooth.CharacteristicUUIDHeartRateMeasurement
	cscServiceUUID   = bluetooth.ServiceUUIDCyclingSpeedAndCadence
	cscCharUUID      = bluetooth.CharacteristicUUIDCSCMeasurement
	powerServiceUUID = bluetooth.New16BitUUID(0x1818)
	powerCharUUID    = bluetooth.New16BitUUID(0x2A63)
	ftmsServiceUUID  = bluetooth.New16BitUUID(0x1826)
	ftmsCharUUID     = bluetooth.New16BitUUID(0x2AD2)
	adapter          = bluetooth.DefaultAdapter
)

type deviceCache struct {
	HR      string `json:"hr,omitempty"`
	Cadence string `json:"cadence,omitempty"`
	Speed   string `json:"speed,omitempty"`
	Power   string `json:"power,omitempty"`
	FTMS    string `json:"ftms,omitempty"`
}

func loadDeviceCache() deviceCache {
	home, err := os.UserHomeDir()
	if err != nil {
		return deviceCache{}
	}
	data, err := os.ReadFile(filepath.Join(home, ".spin_hud_devices.json"))
	if err != nil {
		return deviceCache{}
	}
	var cache deviceCache
	_ = json.Unmarshal(data, &cache)
	return cache
}

func saveDeviceCache(c deviceCache) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(home, ".spin_hud_devices.json"), data, 0644)
}

type bleManager struct {
	mu          sync.Mutex
	activeHR    *bluetooth.Device
	activeCad   *bluetooth.Device
	activeSpd   *bluetooth.Device
	activePower *bluetooth.Device
	activeFTMS  *bluetooth.Device
	connecting  map[string]bool
	cache       deviceCache

	lastCrankNano atomic.Int64
	lastWheelNano atomic.Int64
	lastPowerNano atomic.Int64
}

func newBLEManager() SensorManager {
	return &bleManager{
		connecting: make(map[string]bool),
		cache:      loadDeviceCache(),
	}
}

func (m *bleManager) Scan(d time.Duration) {
	if err := adapter.Enable(); err != nil {
		fmt.Printf("Error enabling Bluetooth adapter: %v\n", err)
		return
	}

	fmt.Printf("Scanning %.0fs — spin the cranks/wheels and start HR broadcast on 965...\n", d.Seconds())
	seen := make(map[string]bool)
	var mu sync.Mutex

	timer := time.AfterFunc(d, func() {
		_ = adapter.StopScan()
	})
	defer timer.Stop()

	err := adapter.Scan(func(a *bluetooth.Adapter, result bluetooth.ScanResult) {
		mu.Lock()
		defer mu.Unlock()

		addr := result.Address.String()
		if seen[addr] {
			return
		}
		seen[addr] = true

		name := result.LocalName()
		uuids := result.ServiceUUIDs()
		var uuidStrs []string
		for _, u := range uuids {
			uuidStrs = append(uuidStrs, u.String())
		}
		mfrList := result.ManufacturerData()
		var mfrStrs []string
		for _, m := range mfrList {
			mfrStrs = append(mfrStrs, fmt.Sprintf("%d", m.CompanyID))
		}
		mfrInfo := ""
		if len(mfrStrs) > 0 {
			mfrInfo = fmt.Sprintf("mfr=[%s]", strings.Join(mfrStrs, ", "))
		}

		fmt.Printf("  %s  rssi=%d  name=%q  %s  %s\n", addr, result.RSSI, name, strings.Join(uuidStrs, " "), mfrInfo)
	})

	if err != nil && !strings.Contains(err.Error(), "canceled") {
		fmt.Printf("Scan error: %v\n", err)
	}
}

type discoveredDev struct {
	result bluetooth.ScanResult
	label  string
}

type discoveredCSC struct {
	result bluetooth.ScanResult
	label  string
	isSpd  bool
	isCad  bool
}

func (m *bleManager) ConnectLoop(state *session.State) {
	if err := adapter.Enable(); err != nil {
		status := fmt.Sprintf("Bluetooth adapter error: %v", err)
		state.UpdateTelemetry(session.Telemetry{Status: &status})
		log.Printf("BLE: failed to enable adapter: %v", err)
		return
	}

	// Attempt fast startup connection using cached device addresses
	m.mu.Lock()
	cachedHR := m.cache.HR
	cachedCad := m.cache.Cadence
	cachedSpd := m.cache.Speed
	cachedPower := m.cache.Power
	cachedFTMS := m.cache.FTMS
	m.mu.Unlock()

	if cachedHR != "" {
		if mac, err := bluetooth.ParseMAC(cachedHR); err == nil {
			go m.connectHR(state, bluetooth.Address{MACAddress: bluetooth.MACAddress{MAC: mac}}, "Garmin HR (cached)")
		}
	}
	if cachedCad != "" {
		if mac, err := bluetooth.ParseMAC(cachedCad); err == nil {
			go m.connectCad(state, bluetooth.Address{MACAddress: bluetooth.MACAddress{MAC: mac}}, "Cadence (cached)")
		}
	}
	if cachedSpd != "" {
		if mac, err := bluetooth.ParseMAC(cachedSpd); err == nil {
			go m.connectSpd(state, bluetooth.Address{MACAddress: bluetooth.MACAddress{MAC: mac}}, "Speed (cached)")
		}
	}
	if cachedPower != "" {
		if mac, err := bluetooth.ParseMAC(cachedPower); err == nil {
			go m.connectPower(state, bluetooth.Address{MACAddress: bluetooth.MACAddress{MAC: mac}}, "Power Meter (cached)")
		}
	}
	if cachedFTMS != "" {
		if mac, err := bluetooth.ParseMAC(cachedFTMS); err == nil {
			go m.connectFTMS(state, bluetooth.Address{MACAddress: bluetooth.MACAddress{MAC: mac}}, "FTMS Trainer (cached)")
		}
	}

	time.Sleep(1 * time.Second)

	for {
		now := time.Now()

		m.mu.Lock()
		activeHR := m.activeHR
		activeCad := m.activeCad
		activeSpd := m.activeSpd
		activePower := m.activePower
		activeFTMS := m.activeFTMS
		connectingHR := m.connecting["hr"]
		connectingCad := m.connecting["cadence"]
		connectingSpd := m.connecting["speed"]
		connectingPower := m.connecting["power"]
		connectingFTMS := m.connecting["ftms"]
		m.mu.Unlock()

		snap := state.GetSnapshot()

		// Stale data watchdog (if pedals or wheel stop rotating)
		lastCrankNano := m.lastCrankNano.Load()
		if activeCad != nil && lastCrankNano > 0 && now.Sub(time.Unix(0, lastCrankNano)) > 3*time.Second && snap.Cadence != nil && *snap.Cadence > 0 {
			zero := 0.0
			state.UpdateTelemetry(session.Telemetry{Cadence: &zero})
		}
		lastWheelNano := m.lastWheelNano.Load()
		if activeSpd != nil && lastWheelNano > 0 && now.Sub(time.Unix(0, lastWheelNano)) > 3*time.Second && snap.SpeedMPH != nil && *snap.SpeedMPH > 0 {
			zero := 0.0
			state.UpdateTelemetry(session.Telemetry{SpeedMPH: &zero})
		}
		lastPowerNano := m.lastPowerNano.Load()
		if (activePower != nil || activeFTMS != nil) && lastPowerNano > 0 && now.Sub(time.Unix(0, lastPowerNano)) > 3*time.Second && snap.Watts > 0 {
			zeroW := 0
			state.UpdateTelemetry(session.Telemetry{PowerWatts: &zeroW})
		}

		// Check disconnects
		if activeHR != nil {
			if connected, err := activeHR.Connected(); err != nil || !connected {
				_ = activeHR.Disconnect()
				m.mu.Lock()
				m.activeHR = nil
				m.mu.Unlock()
				state.SetSensor("hr", false, "Disconnected")
				state.UpdateTelemetry(session.Telemetry{ClearHR: true})
				activeHR = nil
			}
		}
		if activeCad != nil {
			if connected, err := activeCad.Connected(); err != nil || !connected {
				_ = activeCad.Disconnect()
				m.mu.Lock()
				m.activeCad = nil
				m.mu.Unlock()
				state.SetSensor("cadence", false, "Disconnected (spin crank to wake)")
				state.UpdateTelemetry(session.Telemetry{ClearCadence: true})
				activeCad = nil
			}
		}
		if activeSpd != nil {
			if connected, err := activeSpd.Connected(); err != nil || !connected {
				_ = activeSpd.Disconnect()
				m.mu.Lock()
				m.activeSpd = nil
				m.mu.Unlock()
				state.SetSensor("speed", false, "Disconnected (spin wheel to wake)")
				state.UpdateTelemetry(session.Telemetry{ClearSpeed: true})
				activeSpd = nil
			}
		}
		if activePower != nil {
			if connected, err := activePower.Connected(); err != nil || !connected {
				_ = activePower.Disconnect()
				m.mu.Lock()
				m.activePower = nil
				m.mu.Unlock()
				state.SetSensor("power", false, "Disconnected")
				state.UpdateTelemetry(session.Telemetry{ClearPower: true})
				activePower = nil
			}
		}
		if activeFTMS != nil {
			if connected, err := activeFTMS.Connected(); err != nil || !connected {
				_ = activeFTMS.Disconnect()
				m.mu.Lock()
				m.activeFTMS = nil
				m.mu.Unlock()
				state.SetSensor("ftms", false, "Disconnected")
				state.UpdateTelemetry(session.Telemetry{ClearPower: true})
				activeFTMS = nil
			}
		}

		needHR := (activeHR == nil) && !connectingHR
		needCad := (activeCad == nil) && !connectingCad
		needSpd := (activeSpd == nil) && !connectingSpd
		needPower := (activePower == nil) && !connectingPower
		needFTMS := (activeFTMS == nil) && !connectingFTMS
		anyConnecting := connectingHR || connectingCad || connectingSpd || connectingPower || connectingFTMS

		if !needHR && !needCad && !needSpd && !needPower && !needFTMS && !anyConnecting {
			status := "All sensors live"
			state.UpdateTelemetry(session.Telemetry{Status: &status})
			time.Sleep(1500 * time.Millisecond)
			continue
		}

		var missing []string
		if needHR {
			missing = append(missing, "Garmin HR")
		}
		if needCad {
			missing = append(missing, "Cadence (57177)")
		}
		if needSpd {
			missing = append(missing, "Speed (40452)")
		}
		if needPower {
			missing = append(missing, "Power meter")
		}
		if needFTMS {
			missing = append(missing, "FTMS trainer")
		}
		if len(missing) > 0 {
			status := fmt.Sprintf("Searching: %s… (spin pedals/wheel)", strings.Join(missing, ", "))
			state.UpdateTelemetry(session.Telemetry{Status: &status})
		}

		// Perform a short BLE scan to find missing sensors
		var (
			foundHR    []discoveredDev
			foundCSC   []discoveredCSC
			foundPower []discoveredDev
			foundFTMS  []discoveredDev
			scanMu     sync.Mutex
		)

		scanTimer := time.AfterFunc(3*time.Second, func() {
			_ = adapter.StopScan()
		})

		_ = adapter.Scan(func(a *bluetooth.Adapter, result bluetooth.ScanResult) {
			scanMu.Lock()
			defer scanMu.Unlock()

			name := result.LocalName()
			nameLower := strings.ToLower(name)
			addrLower := strings.ToLower(result.Address.String())
			combined := nameLower + " " + addrLower

			hasHR := result.HasServiceUUID(hrServiceUUID)
			if !hasHR {
				for _, mfr := range result.ManufacturerData() {
					if mfr.CompanyID == 135 { // 0x0087 Garmin
						hasHR = true
						break
					}
				}
			}
			if !hasHR {
				for _, kw := range []string{"forerunner", "garmin", "fenix", "instinct", "965", "hr-", "heartrate"} {
					if strings.Contains(nameLower, kw) {
						hasHR = true
						break
					}
				}
			}

			if hasHR {
				lbl := name
				if lbl == "" {
					lbl = result.Address.String()
				}
				foundHR = append(foundHR, discoveredDev{result: result, label: lbl})
			}

			hasCSC := result.HasServiceUUID(cscServiceUUID)
			if !hasCSC {
				for _, kw := range []string{"magene", "s3", "57177", "40452", "cadence", "speed", "csc", "spd", "cad"} {
					if strings.Contains(combined, kw) {
						hasCSC = true
						break
					}
				}
			}

			if hasCSC {
				isSpd := false
				for _, kw := range []string{"40452", "spd", "speed", "wheel"} {
					if strings.Contains(combined, kw) {
						isSpd = true
						break
					}
				}
				isCad := false
				for _, kw := range []string{"57177", "cad", "cadence", "crank"} {
					if strings.Contains(combined, kw) {
						isCad = true
						break
					}
				}
				lbl := name
				if lbl == "" {
					lbl = result.Address.String()
				}
				foundCSC = append(foundCSC, discoveredCSC{
					result: result,
					label:  lbl,
					isSpd:  isSpd,
					isCad:  isCad,
				})
			}

			// Power Meter (0x1818)
			hasPower := result.HasServiceUUID(powerServiceUUID)
			if !hasPower {
				for _, kw := range []string{"stages", "assioma", "4iiii", "quarq", "power meter", "cycling power", "rally", "vector"} {
					if strings.Contains(nameLower, kw) {
						hasPower = true
						break
					}
				}
			}
			if hasPower {
				lbl := name
				if lbl == "" {
					lbl = result.Address.String()
				}
				foundPower = append(foundPower, discoveredDev{result: result, label: lbl})
			}

			// FTMS Indoor Trainer (0x1826)
			hasFTMS := result.HasServiceUUID(ftmsServiceUUID)
			if !hasFTMS {
				for _, kw := range []string{"kickr", "tacx", "wahoo", "neo", "suito", "direto", "flux", "saris", "ftms", "trainer", "smart bike", "indoor bike"} {
					if strings.Contains(nameLower, kw) {
						hasFTMS = true
						break
					}
				}
			}
			if hasFTMS {
				lbl := name
				if lbl == "" {
					lbl = result.Address.String()
				}
				foundFTMS = append(foundFTMS, discoveredDev{result: result, label: lbl})
			}
		})
		scanTimer.Stop()

		// Dispatch HR connection
		if needHR && len(foundHR) > 0 {
			target := foundHR[0]
			needHR = false
			go m.connectHR(state, target.result.Address, target.label)
		}

		// Dispatch Power connection
		if needPower && len(foundPower) > 0 {
			target := foundPower[0]
			needPower = false
			go m.connectPower(state, target.result.Address, target.label)
		}

		// Dispatch FTMS connection
		if needFTMS && len(foundFTMS) > 0 {
			target := foundFTMS[0]
			needFTMS = false
			go m.connectFTMS(state, target.result.Address, target.label)
		}

		// Dispatch CSC connections: explicit matching first
		assigned := make(map[string]bool)
		for _, dev := range foundCSC {
			addr := dev.result.Address.String()
			if needSpd && dev.isSpd && !dev.isCad {
				needSpd = false
				assigned[addr] = true
				go m.connectSpd(state, dev.result.Address, dev.label)
			} else if needCad && dev.isCad && !dev.isSpd {
				needCad = false
				assigned[addr] = true
				go m.connectCad(state, dev.result.Address, dev.label)
			}
		}

		// Fallback for remaining unassigned CSC sensors
		for _, dev := range foundCSC {
			addr := dev.result.Address.String()
			if assigned[addr] {
				continue
			}
			m.mu.Lock()
			activeAddrs := make(map[string]bool)
			if m.activeCad != nil {
				activeAddrs[m.activeCad.Address.String()] = true
			}
			if m.activeSpd != nil {
				activeAddrs[m.activeSpd.Address.String()] = true
			}
			m.mu.Unlock()

			if activeAddrs[addr] {
				continue
			}

			if needCad {
				needCad = false
				assigned[addr] = true
				go m.connectCad(state, dev.result.Address, dev.label)
			} else if needSpd {
				needSpd = false
				assigned[addr] = true
				go m.connectSpd(state, dev.result.Address, dev.label)
			}
		}

		time.Sleep(1500 * time.Millisecond)
	}
}

func (m *bleManager) connectHR(state *session.State, addr bluetooth.Address, label string) {
	m.mu.Lock()
	if m.connecting["hr"] {
		m.mu.Unlock()
		return
	}
	m.connecting["hr"] = true
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.connecting, "hr")
		m.mu.Unlock()
	}()

	state.SetSensor("hr", false, fmt.Sprintf("Connecting %s…", label))

	dev, err := adapter.Connect(addr, bluetooth.ConnectionParams{})
	if err != nil {
		state.SetSensor("hr", false, fmt.Sprintf("Drop: %v", err))
		return
	}

	svcs, err := dev.DiscoverServices([]bluetooth.UUID{hrServiceUUID})
	if err != nil || len(svcs) == 0 {
		_ = dev.Disconnect()
		state.SetSensor("hr", false, "HR Service not found")
		return
	}

	chars, err := svcs[0].DiscoverCharacteristics([]bluetooth.UUID{hrCharUUID})
	if err != nil || len(chars) == 0 {
		_ = dev.Disconnect()
		state.SetSensor("hr", false, "HR Characteristic not found")
		return
	}

	err = chars[0].EnableNotifications(func(buf []byte) {
		bpm, ok := ParseHR(buf)
		if ok {
			state.UpdateTelemetry(session.Telemetry{HR: &bpm})
		}
	})
	if err != nil {
		_ = dev.Disconnect()
		state.SetSensor("hr", false, fmt.Sprintf("Notify error: %v", err))
		return
	}

	m.mu.Lock()
	m.activeHR = &dev
	m.cache.HR = addr.String()
	saveDeviceCache(m.cache)
	m.mu.Unlock()

	state.SetSensor("hr", true, label)
	log.Printf("BLE: Connected HR: %s (%s)", label, addr.String())
}

func (m *bleManager) connectCad(state *session.State, addr bluetooth.Address, label string) {
	m.mu.Lock()
	if m.connecting["cadence"] {
		m.mu.Unlock()
		return
	}
	m.connecting["cadence"] = true
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.connecting, "cadence")
		m.mu.Unlock()
	}()

	state.SetSensor("cadence", false, fmt.Sprintf("Connecting %s…", label))

	dev, err := adapter.Connect(addr, bluetooth.ConnectionParams{})
	if err != nil {
		state.SetSensor("cadence", false, fmt.Sprintf("Drop: %v", err))
		return
	}

	svcs, err := dev.DiscoverServices([]bluetooth.UUID{cscServiceUUID})
	if err != nil || len(svcs) == 0 {
		_ = dev.Disconnect()
		state.SetSensor("cadence", false, "CSC Service not found")
		return
	}

	chars, err := svcs[0].DiscoverCharacteristics([]bluetooth.UUID{cscCharUUID})
	if err != nil || len(chars) == 0 {
		_ = dev.Disconnect()
		state.SetSensor("cadence", false, "CSC Characteristic not found")
		return
	}

	var crankRef *CSCRef
	var refMu sync.Mutex

	err = chars[0].EnableNotifications(func(buf []byte) {
		refMu.Lock()
		rpm, ok, newRef := ParseCSCCrank(buf, crankRef)
		crankRef = newRef
		refMu.Unlock()

		if ok {
			m.lastCrankNano.Store(time.Now().UnixNano())
			state.UpdateTelemetry(session.Telemetry{Cadence: &rpm})
		}
	})
	if err != nil {
		_ = dev.Disconnect()
		state.SetSensor("cadence", false, fmt.Sprintf("Notify error: %v", err))
		return
	}

	m.mu.Lock()
	m.activeCad = &dev
	m.cache.Cadence = addr.String()
	saveDeviceCache(m.cache)
	m.mu.Unlock()

	state.SetSensor("cadence", true, label)
	log.Printf("BLE: Connected Cadence: %s (%s)", label, addr.String())
}

func (m *bleManager) connectSpd(state *session.State, addr bluetooth.Address, label string) {
	m.mu.Lock()
	if m.connecting["speed"] {
		m.mu.Unlock()
		return
	}
	m.connecting["speed"] = true
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.connecting, "speed")
		m.mu.Unlock()
	}()

	state.SetSensor("speed", false, fmt.Sprintf("Connecting %s…", label))

	dev, err := adapter.Connect(addr, bluetooth.ConnectionParams{})
	if err != nil {
		state.SetSensor("speed", false, fmt.Sprintf("Drop: %v", err))
		return
	}

	svcs, err := dev.DiscoverServices([]bluetooth.UUID{cscServiceUUID})
	if err != nil || len(svcs) == 0 {
		_ = dev.Disconnect()
		state.SetSensor("speed", false, "CSC Service not found")
		return
	}

	chars, err := svcs[0].DiscoverCharacteristics([]bluetooth.UUID{cscCharUUID})
	if err != nil || len(chars) == 0 {
		_ = dev.Disconnect()
		state.SetSensor("speed", false, "CSC Characteristic not found")
		return
	}

	var wheelRef *CSCRef
	var refMu sync.Mutex

	err = chars[0].EnableNotifications(func(buf []byte) {
		refMu.Lock()
		spdMPH, deltaMi, newRef := ParseCSCWheel(buf, wheelRef, state.WheelCirc())
		wheelRef = newRef
		refMu.Unlock()

		m.lastWheelNano.Store(time.Now().UnixNano())
		state.AddDistanceDelta(deltaMi)
		state.UpdateTelemetry(session.Telemetry{SpeedMPH: &spdMPH})
	})
	if err != nil {
		_ = dev.Disconnect()
		state.SetSensor("speed", false, fmt.Sprintf("Notify error: %v", err))
		return
	}

	m.mu.Lock()
	m.activeSpd = &dev
	m.cache.Speed = addr.String()
	saveDeviceCache(m.cache)
	m.mu.Unlock()

	state.SetSensor("speed", true, label)
	log.Printf("BLE: Connected Speed: %s (%s)", label, addr.String())
}

func (m *bleManager) connectPower(state *session.State, addr bluetooth.Address, label string) {
	m.mu.Lock()
	if m.connecting["power"] {
		m.mu.Unlock()
		return
	}
	m.connecting["power"] = true
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.connecting, "power")
		m.mu.Unlock()
	}()

	state.SetSensor("power", false, fmt.Sprintf("Connecting %s…", label))

	dev, err := adapter.Connect(addr, bluetooth.ConnectionParams{})
	if err != nil {
		state.SetSensor("power", false, fmt.Sprintf("Drop: %v", err))
		return
	}

	svcs, err := dev.DiscoverServices([]bluetooth.UUID{powerServiceUUID})
	if err != nil || len(svcs) == 0 {
		_ = dev.Disconnect()
		state.SetSensor("power", false, "Power Service not found")
		return
	}

	chars, err := svcs[0].DiscoverCharacteristics([]bluetooth.UUID{powerCharUUID})
	if err != nil || len(chars) == 0 {
		_ = dev.Disconnect()
		state.SetSensor("power", false, "Power Characteristic not found")
		return
	}

	err = chars[0].EnableNotifications(func(buf []byte) {
		watts, ok := ParseCyclingPower(buf)
		if ok {
			m.lastPowerNano.Store(time.Now().UnixNano())
			pSrc := "meter"
			state.UpdateTelemetry(session.Telemetry{
				PowerWatts:  &watts,
				PowerSource: &pSrc,
			})
		}
	})
	if err != nil {
		_ = dev.Disconnect()
		state.SetSensor("power", false, fmt.Sprintf("Notify error: %v", err))
		return
	}

	m.mu.Lock()
	m.activePower = &dev
	m.cache.Power = addr.String()
	saveDeviceCache(m.cache)
	m.mu.Unlock()

	state.SetSensor("power", true, label)
	log.Printf("BLE: Connected Power Meter: %s (%s)", label, addr.String())
}

func (m *bleManager) connectFTMS(state *session.State, addr bluetooth.Address, label string) {
	m.mu.Lock()
	if m.connecting["ftms"] {
		m.mu.Unlock()
		return
	}
	m.connecting["ftms"] = true
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.connecting, "ftms")
		m.mu.Unlock()
	}()

	state.SetSensor("ftms", false, fmt.Sprintf("Connecting %s…", label))

	dev, err := adapter.Connect(addr, bluetooth.ConnectionParams{})
	if err != nil {
		state.SetSensor("ftms", false, fmt.Sprintf("Drop: %v", err))
		return
	}

	svcs, err := dev.DiscoverServices([]bluetooth.UUID{ftmsServiceUUID})
	if err != nil || len(svcs) == 0 {
		_ = dev.Disconnect()
		state.SetSensor("ftms", false, "FTMS Service not found")
		return
	}

	chars, err := svcs[0].DiscoverCharacteristics([]bluetooth.UUID{ftmsCharUUID})
	if err != nil || len(chars) == 0 {
		_ = dev.Disconnect()
		state.SetSensor("ftms", false, "FTMS Characteristic not found")
		return
	}

	err = chars[0].EnableNotifications(func(buf []byte) {
		data, ok := ParseFTMSIndoorBike(buf)
		if ok {
			now := time.Now().UnixNano()
			pSrc := "ftms"
			t := session.Telemetry{
				PowerSource: &pSrc,
			}
			if data.PowerWatts != nil {
				m.lastPowerNano.Store(now)
				t.PowerWatts = data.PowerWatts
			}
			if data.CadenceRPM != nil {
				m.lastCrankNano.Store(now)
				t.Cadence = data.CadenceRPM
			}
			if data.SpeedMPH != nil {
				m.lastWheelNano.Store(now)
				t.SpeedMPH = data.SpeedMPH
			}
			if data.HeartRate != nil {
				t.HR = data.HeartRate
			}
			state.UpdateTelemetry(t)
		}
	})
	if err != nil {
		_ = dev.Disconnect()
		state.SetSensor("ftms", false, fmt.Sprintf("Notify error: %v", err))
		return
	}

	m.mu.Lock()
	m.activeFTMS = &dev
	m.cache.FTMS = addr.String()
	saveDeviceCache(m.cache)
	m.mu.Unlock()

	state.SetSensor("ftms", true, label)
	log.Printf("BLE: Connected FTMS Trainer: %s (%s)", label, addr.String())
}

