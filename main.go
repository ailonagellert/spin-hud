package main

import (
	"embed"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"syscall"
	"time"

	"spin-hud/internal/ble"
	"spin-hud/internal/server"
	"spin-hud/internal/session"
)

//go:embed web/index.html
var webFS embed.FS

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

func extractPlaylistID(s string) string {
	if idx := indexOf(s, "list="); idx >= 0 {
		rest := s[idx+len("list="):]
		for i := 0; i < len(rest); i++ {
			if rest[i] == '&' {
				return rest[:i]
			}
		}
		return rest
	}
	return s
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func main() {
	scan := flag.Bool("scan", false, "Scan nearby BLE sensors")
	selfCheck := flag.Bool("self-check", false, "Run parser & engine validation")
	port := flag.Int("port", 8080, "Web server port (default: 8080)")
	lan := flag.Bool("lan", false, "Listen on 0.0.0.0 for LAN access (default: 127.0.0.1)")
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

	pl := extractPlaylistID(*playlist)
	host := "127.0.0.1"
	if *lan {
		host = "0.0.0.0"
	}

	state := session.NewState(pl)
	indexHTML, err := webFS.ReadFile("web/index.html")
	if err != nil {
		log.Fatalf("embedded UI missing: %v", err)
	}

	srv := server.New(state, string(indexHTML))

	listener, err := server.Listen(host, *port)
	if err != nil {
		if isAddrInUse(err) {
			url := fmt.Sprintf("http://localhost:%d", *port)
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
	if host != "127.0.0.1" && host != "0.0.0.0" {
		displayHost = host
	}
	url := fmt.Sprintf("http://%s:%d", displayHost, *port)
	fmt.Println("========================================================")
	fmt.Printf("  SPIN STUDIO LIVE: %s\n", url)
	fmt.Printf("  Playlist: https://youtube.com/playlist?list=%s\n", state.PlaylistID)
	fmt.Println("  Sensors : Garmin 965 (HR) + Magene (Cadence & Speed)")
	fmt.Println("========================================================")

	if !*noBrowser {
		go func() {
			time.Sleep(1 * time.Second)
			openBrowser(url)
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
