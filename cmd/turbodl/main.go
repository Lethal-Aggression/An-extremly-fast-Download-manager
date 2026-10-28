package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Lethal-Aggression/turbodl/pkg/bond"
	"github.com/Lethal-Aggression/turbodl/pkg/config"
	"github.com/Lethal-Aggression/turbodl/pkg/engine"
	"github.com/Lethal-Aggression/turbodl/pkg/ui"
	"github.com/Lethal-Aggression/turbodl/pkg/web"
)

type hotPlugNotifier struct {
	lastAlert string
	alertTime time.Time
}

func (h *hotPlugNotifier) OnAdapterConnected(adapter *bond.Adapter) {
	if adapter.IsPhone {
		h.lastAlert = fmt.Sprintf("[HOT-PLUG] PHONE DETECTED: %s (%s) -> BONDED!", adapter.DisplayName, adapter.IP)
	} else {
		h.lastAlert = fmt.Sprintf("[HOT-PLUG] ADAPTER DETECTED: %s (%s) -> ADDED!", adapter.DisplayName, adapter.IP)
	}
	h.alertTime = time.Now()
}

func (h *hotPlugNotifier) OnAdapterDisconnected(adapter *bond.Adapter) {
	h.lastAlert = fmt.Sprintf("[HOT-UNPLUG] DISCONNECTED: %s", adapter.DisplayName)
	h.alertTime = time.Now()
}

func (h *hotPlugNotifier) GetActiveAlert() string {
	if time.Since(h.alertTime) < 5*time.Second {
		return h.lastAlert
	}
	return ""
}

func main() {
	ui.EnableVirtualTerminalProcessing()

	// Disable default timestamp logger when in TUI mode to keep interface clean
	log.SetFlags(0)
	log.SetOutput(os.Stderr)

	cfg := config.LoadConfig("turbodl_config.json")
	pool := bond.NewAdapterPool()
	monitor := bond.NewMonitor(pool, 1200*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	notifier := &hotPlugNotifier{}
	monitor.AddListener(notifier)
	monitor.Start(ctx)

	// Clean exit handler
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Print(ui.ShowCursor + ui.Reset + "\n")
		os.Exit(0)
	}()

	// Handle direct CLI URL argument
	args := os.Args[1:]
	if len(args) > 0 {
		if args[0] == "--web" || args[0] == "-w" {
			// Web server mode
			srv := web.NewServer(cfg, pool, monitor)
			_ = srv.Start()
			return
		}

		if strings.HasPrefix(args[0], "http://") || strings.HasPrefix(args[0], "https://") {
			runLiveDownload(args[0], "", cfg, pool, monitor, notifier)
			return
		}
	}

	// Interactive Minimalist Menu Mode
	runInteractiveMenu(cfg, pool, monitor, notifier)
}

func runInteractiveMenu(cfg *config.AppConfig, pool *bond.AdapterPool, monitor *bond.Monitor, notifier *hotPlugNotifier) {
	reader := bufio.NewReader(os.Stdin)

	for {
		adapters := pool.GetAll()
		fmt.Print(ui.RenderMainMenu(adapters, false))

		input, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		choice := strings.TrimSpace(input)

		switch choice {
		case "1":
			fmt.Print(ui.ClearScreen)
			fmt.Println(ui.BoldWhite + ui.Line(ui.TopLeft, ui.Horizontal, ui.TopRight, ui.BoxWidth) + ui.Reset)
			fmt.Printf("%s%s  %sENTER TARGET DOWNLOAD URL:%s%s\n", ui.BoldWhite, ui.Vertical, ui.InverseBold, ui.Reset, ui.Vertical)
			fmt.Println(ui.BoldWhite + ui.Line(ui.BottomLeft, ui.Horizontal, ui.BottomRight, ui.BoxWidth) + ui.Reset)
			fmt.Print("\n " + ui.BoldWhite + "URL ▶ " + ui.Reset)
			
			rawURL, _ := reader.ReadString('\n')
			rawURL = strings.TrimSpace(rawURL)
			if rawURL == "" {
				continue
			}

			fmt.Print("\n " + ui.BoldWhite + "CUSTOM FILENAME (press enter to auto-detect) ▶ " + ui.Reset)
			customName, _ := reader.ReadString('\n')
			customName = strings.TrimSpace(customName)

			runLiveDownload(rawURL, customName, cfg, pool, monitor, notifier)

		case "2":
			runHardwareMonitorLoop(pool, monitor, reader)

		case "3":
			runSpeedBenchmark(pool, reader)

		case "4":
			runSettingsMenu(cfg, reader)

		case "0", "exit", "quit":
			fmt.Print(ui.ShowCursor + ui.Reset + "\n")
			os.Exit(0)

		default:
			time.Sleep(300 * time.Millisecond)
		}
	}
}

func runLiveDownload(rawURL, filename string, cfg *config.AppConfig, pool *bond.AdapterPool, monitor *bond.Monitor, notifier *hotPlugNotifier) {
	fmt.Print(ui.ClearScreen + ui.HideCursor)
	defer fmt.Print(ui.ShowCursor)

	d := engine.NewDownloader(rawURL, cfg.DownloadDir, filename, cfg, pool, monitor)

	fmt.Println(ui.BoldWhite + "\n [CONNECTING] Inspecting server and checking multi-interface support..." + ui.Reset)
	ctx := context.Background()
	if err := d.Prepare(ctx); err != nil {
		fmt.Printf("\n %s ERROR: %v %s\n\n", ui.InverseBold, err, ui.Reset)
		fmt.Println("Press Enter to return...")
		bufio.NewReader(os.Stdin).ReadString('\n')
		return
	}

	if err := d.Start(ctx); err != nil {
		fmt.Printf("\n %s ERROR STARTING DOWNLOAD: %v %s\n\n", ui.InverseBold, err, ui.Reset)
		fmt.Println("Press Enter to return...")
		bufio.NewReader(os.Stdin).ReadString('\n')
		return
	}

	// Live rendering loop (refresh 10 times a second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		snapshot := d.Snapshot()
		alert := notifier.GetActiveAlert()
		
		fmt.Print(ui.RenderLiveDownload(snapshot, pool, alert))

		if snapshot.Status == engine.StatusCompleted {
			fmt.Printf("\n\n %s [DONE] FILE SUCCESSFULLY DOWNLOADED TO: %s %s\n\n", ui.InverseBold, snapshot.DestPath, ui.Reset)
			fmt.Println("Press Enter to continue...")
			bufio.NewReader(os.Stdin).ReadString('\n')
			return
		}

		if snapshot.Status == engine.StatusFailed {
			fmt.Printf("\n\n %s [FAILED] DOWNLOAD ERROR ENCOUNTERED %s\n\n", ui.InverseBold, ui.Reset)
			fmt.Println("Press Enter to continue...")
			bufio.NewReader(os.Stdin).ReadString('\n')
			return
		}
	}
}

func runHardwareMonitorLoop(pool *bond.AdapterPool, monitor *bond.Monitor, reader *bufio.Reader) {
	fmt.Print(ui.ClearScreen + ui.HideCursor)
	defer fmt.Print(ui.ShowCursor)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	stopChan := make(chan struct{})
	go func() {
		_, _ = reader.ReadString('\n')
		close(stopChan)
	}()

	for {
		select {
		case <-stopChan:
			return
		case <-ticker.C:
			adapters := pool.GetAll()
			var b strings.Builder
			b.WriteString("\033[H")
			b.WriteString(ui.BoldWhite + ui.Line(ui.TopLeft, ui.Horizontal, ui.TopRight, ui.BoxWidth) + "\n" + ui.Reset)
			b.WriteString(fmt.Sprintf("%s%s  TURBODL // LIVE HARDWARE HOT-PLUG WATCHER                    %s%s\n", ui.BoldWhite, ui.Vertical, ui.Vertical, ui.Reset))
			b.WriteString(fmt.Sprintf("%s%s  %sSTATUS: MONITORING USB AND NETWORK BUS%s                           %s%s\n", ui.BoldWhite, ui.Vertical, ui.InverseBold, ui.Reset, ui.Vertical, ui.Reset))
			b.WriteString(ui.BoldWhite + ui.Line(ui.TeeLeft, ui.Horizontal, ui.TeeRight, ui.BoxWidth) + "\n" + ui.Reset)

			b.WriteString(fmt.Sprintf("%s%s  Plug in your phone with USB cable and enable USB Tethering.          %s%s\n", ui.BoldWhite, ui.Vertical, ui.Vertical, ui.Reset))
			b.WriteString(fmt.Sprintf("%s%s  TurboDL binds newly attached devices in real-time.                   %s%s\n", ui.BoldWhite, ui.Vertical, ui.Vertical, ui.Reset))
			b.WriteString(ui.BoldWhite + ui.Line(ui.TeeLeft, ui.Horizontal, ui.TeeRight, ui.BoxWidth) + "\n" + ui.Reset)

			b.WriteString(fmt.Sprintf("%s%s  %-20s %-16s %-18s %-10s%s%s\n", ui.BoldWhite, ui.Vertical, "DEVICE", "INTERFACE", "LOCAL IP", "STATUS", ui.Vertical, ui.Reset))
			for _, a := range adapters {
				label := "[WIFI] PC Wi-Fi"
				if a.IsPhone {
					label = "[PHONE] Tether"
				}
				row := fmt.Sprintf("  %-20s %-16s %-18s %sBONDED%s", label, a.Name, a.IP.String(), ui.InverseBold, ui.Reset)
				b.WriteString(fmt.Sprintf("%s%s %-79s%s%s\n", ui.BoldWhite, ui.Vertical, row, ui.Vertical, ui.Reset))
			}

			b.WriteString(ui.BoldWhite + ui.Line(ui.BottomLeft, ui.Horizontal, ui.BottomRight, ui.BoxWidth) + "\n" + ui.Reset)
			b.WriteString(fmt.Sprintf(" %sPress [ENTER] to return to menu...%s\n", ui.MutedGray, ui.Reset))
			fmt.Print(b.String())
		}
	}
}

func runSpeedBenchmark(pool *bond.AdapterPool, reader *bufio.Reader) {
	fmt.Print(ui.ClearScreen)
	fmt.Println(ui.BoldWhite + ui.Line(ui.TopLeft, ui.Horizontal, ui.TopRight, ui.BoxWidth) + ui.Reset)
	fmt.Printf("%s%s  %sTESTING CONNECTIVITY & BANDWIDTH ON ALL ACTIVE INTERFACES...%s%s\n", ui.BoldWhite, ui.Vertical, ui.InverseBold, ui.Reset, ui.Vertical)
	fmt.Println(ui.BoldWhite + ui.Line(ui.BottomLeft, ui.Horizontal, ui.BottomRight, ui.BoxWidth) + ui.Reset)

	adapters := pool.GetAll()
	for _, a := range adapters {
		fmt.Printf("\n [*] Probing adapter: %s (%s)...\n", a.DisplayName, a.IP)
		ok := a.TestConnectivity("http://www.google.com/generate_204", 3*time.Second)
		if ok {
			fmt.Printf("   %s ONLINE & ROUTING VERIFIED %s\n", ui.InverseBold, ui.Reset)
		} else {
			fmt.Printf("   %s NO ROUTE / TIMEOUT %s\n", ui.DimWhite, ui.Reset)
		}
	}

	fmt.Println("\nPress Enter to return to menu...")
	reader.ReadString('\n')
}

func runSettingsMenu(cfg *config.AppConfig, reader *bufio.Reader) {
	fmt.Print(ui.ClearScreen)
	fmt.Println(ui.BoldWhite + ui.Line(ui.TopLeft, ui.Horizontal, ui.TopRight, ui.BoxWidth) + ui.Reset)
	fmt.Printf("%s%s  TURBODL // SETTINGS & PERFORMANCE TUNING                             %s%s\n", ui.BoldWhite, ui.Vertical, ui.Vertical, ui.Reset)
	fmt.Println(ui.BoldWhite + ui.Line(ui.TeeLeft, ui.Horizontal, ui.TeeRight, ui.BoxWidth) + ui.Reset)
	fmt.Printf("%s%s  [1] Download Directory:    %-42s%s%s\n", ui.BoldWhite, ui.Vertical, cfg.DownloadDir, ui.Vertical, ui.Reset)
	fmt.Printf("%s%s  [2] Chunk Block Size:      %-42d%s%s\n", ui.BoldWhite, ui.Vertical, cfg.ChunkSizeMB, ui.Vertical, ui.Reset)
	fmt.Printf("%s%s  [3] Threads Per Adapter:   %-42d%s%s\n", ui.BoldWhite, ui.Vertical, cfg.ThreadsPerAdapter, ui.Vertical, ui.Reset)
	fmt.Printf("%s%s  [4] Auto-Bond All Devices: %-42t%s%s\n", ui.BoldWhite, ui.Vertical, cfg.AutoBondAll, ui.Vertical, ui.Reset)
	fmt.Printf("%s%s  [0] Save & Return                                                   %s%s\n", ui.BoldWhite, ui.Vertical, ui.Vertical, ui.Reset)
	fmt.Println(ui.BoldWhite + ui.Line(ui.BottomLeft, ui.Horizontal, ui.BottomRight, ui.BoxWidth) + ui.Reset)

	fmt.Print("\nSELECT OPTION ▶ ")
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	switch choice {
	case "1":
		fmt.Print("Enter new download directory path: ")
		p, _ := reader.ReadString('\n')
		p = strings.TrimSpace(p)
		if p != "" {
			cfg.DownloadDir = p
		}
	case "2":
		fmt.Print("Enter chunk size in MB (e.g. 4, 8, 16): ")
		var sz int
		_, _ = fmt.Scanf("%d\n", &sz)
		if sz > 0 {
			cfg.ChunkSizeMB = sz
		}
	case "3":
		fmt.Print("Enter threads per adapter (e.g. 4, 8): ")
		var t int
		_, _ = fmt.Scanf("%d\n", &t)
		if t > 0 {
			cfg.ThreadsPerAdapter = t
		}
	case "4":
		cfg.AutoBondAll = !cfg.AutoBondAll
	}

	_ = cfg.Save("turbodl_config.json")
}
