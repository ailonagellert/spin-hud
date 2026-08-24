package ble

import (
	"fmt"
	"log"
	"time"

	"spin-hud/internal/session"
)

// stubManager is a placeholder until the tinygo-org/bluetooth implementation
// lands (milestone 3). It keeps the tree compiling and the HTTP server useful.
type stubManager struct{}

func newManager() SensorManager { return &stubManager{} }

func (m *stubManager) ConnectLoop(state *session.State) {
	status := "Sensors not available in this build (BLE lands in milestone 3)"
	state.UpdateTelemetry(session.Telemetry{Status: &status})
	log.Println("BLE: stub manager active; no sensor connections")
	for {
		time.Sleep(time.Hour)
	}
}

func (m *stubManager) Scan(d time.Duration) {
	fmt.Println("BLE scan not available in this build (milestone 3)")
}
