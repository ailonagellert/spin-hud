package main

import (
	"crypto/rand"
	"embed"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"spin-hud/internal/ble"
	"spin-hud/internal/db"
	"spin-hud/internal/server"
	"spin-hud/internal/session"
	"spin-hud/internal/strava"
)

//go:embed web/index.html web/launcher.html
var webFS embed.FS

func generatePIN() string {
	n, err := rand.Int(rand.Reader, big.NewInt(900000))
	if err != nil {
		return "849201"
	}
	return fmt.Sprintf("%06d", n.Int64()+100000)
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "localhost"
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String()
			}
		}
	}
	return "localhost"
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("failed to open browser: %v", err)
	}
}

func main() {
	scan := flag.Bool("scan", false, "Scan nearby BLE sensors")
	selfCheck := flag.Bool("self-check", false, "Run parser & engine validation")
	port := flag.Int("port", 8080, "Web server port (default: 8080)")
	lan := flag.Bool("lan", false, "Listen on 0.0.0.0 for LAN access (default: 127.0.0.1)")
	pinFlag := flag.String("pin", "", "Custom LAN pairing PIN (auto-generated if --lan enabled)")
	dbPath := flag.String("db", "", "Path to SQLite database file")
	playlist := flag.String("playlist", session.DefaultPlaylistID, "YouTube Playlist ID or URL")
	noBrowser := flag.Bool("no-browser", false, "Do not automatically open browser")
	flag.Parse()

	if *selfCheck {
		if ble.SelfCheck() != 0 {
			log.Fatal("self-check failed")
		}
		fmt.Println("self-check OK")
		return
	}

	if *scan {
		ble.Scan(8 * time.Second)
		return
	}

	pl := session.ExtractPlaylistID(*playlist)
	host := "127.0.0.1"
	lanPIN := ""
	if *lan {
		host = "0.0.0.0"
		if *pinFlag != "" {
			lanPIN = *pinFlag
		} else {
			lanPIN = generatePIN()
		}
	}

	state := session.NewState(pl)
	indexHTML, err := webFS.ReadFile("web/index.html")
	if err != nil {
		log.Fatalf("embedded UI missing: %v", err)
	}
	launcherHTML, err := webFS.ReadFile("web/launcher.html")
	if err != nil {
		log.Fatalf("embedded launcher missing: %v", err)
	}

	exePath, exeErr := os.Executable()
	base := "."
	if exeErr == nil {
		base = filepath.Dir(exePath)
	}

	resolvedDBPath := *dbPath
	if resolvedDBPath == "" {
		resolvedDBPath = filepath.Join(base, "spin_hud.db")
	}
	database, err := db.Open(resolvedDBPath)
	if err != nil {
		log.Printf("warning: sqlite persistence unavailable: %v", err)
	} else {
		defer database.Close()
	}

	sc := strava.New(
		filepath.Join(base, "strava-app.json"),
		filepath.Join(base, "strava-tokens.json"),
		fmt.Sprintf("http://localhost:%d/api/strava/callback", *port),
	)
	srv := server.New(state, string(indexHTML), string(launcherHTML), sc, database, lanPIN)

	listener, err := server.Listen(host, *port)
	if err != nil {
		if isAddrInUse(err) {
			url := fmt.Sprintf("http://localhost:%d/launcher", *port)
			fmt.Println("========================================================")
			fmt.Printf("  [INFO] Spin Studio is already running at %s\n", url)
			fmt.Println("  Opening active session in your browser...")
			fmt.Println("========================================================")
			if !*noBrowser {
				openBrowser(url)
			}
			return
		}
		log.Fatalf("listen failed: %v", err)
	}

	displayHost := "localhost"
	if host == "0.0.0.0" {
		displayHost = getLocalIP()
	} else if host != "127.0.0.1" {
		displayHost = host
	}
	url := fmt.Sprintf("http://%s:%d/launcher", displayHost, *port)
	fmt.Println("========================================================")
	fmt.Printf("  SPIN STUDIO LIVE: %s\n", url)
	if *lan {
		fmt.Printf("  LAN Pairing PIN : %s\n", lanPIN)
	}
	fmt.Printf("  Playlist: https://youtube.com/playlist?list=%s\n", state.PlaylistID)
	fmt.Println("  Sensors : Garmin 965 (HR) + Magene + Power / FTMS")
	fmt.Println("  Storage : SQLite History & FIT / TCX Export Enabled")
	fmt.Println("========================================================")

	if !*noBrowser {
		go func() {
			time.Sleep(1 * time.Second)
			openBrowser(fmt.Sprintf("http://localhost:%d/launcher", *port))
		}()
	}

	// BLE connect loop runs alongside the HTTP server.
	go ble.ConnectLoop(state)

	if err := http.Serve(listener, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}

func isAddrInUse(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if errno, ok := opErr.Err.(syscall.Errno); ok {
			// 10048 = WSAEADDRINUSE (Windows), EADDRINUSE = 98/48 (Linux/macOS)
			return errno == 10048 || errno == syscall.EADDRINUSE
		}
	}
	return false
}
