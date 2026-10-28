package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/Lethal-Aggression/turbodl/pkg/bond"
	"github.com/Lethal-Aggression/turbodl/pkg/engine"
)

// Monochromatic Minimalist Style Tokens (Pure Black & White with Sharp Contrast)
const (
	Reset       = "\033[0m"
	BoldWhite   = "\033[1;97m"
	DimWhite    = "\033[37m"
	MutedGray   = "\033[90m"
	Inverse     = "\033[7m"       // Black on White
	InverseBold = "\033[7;1m"
	Underline   = "\033[4m"
	ClearScreen = "\033[2J\033[H"
	HideCursor  = "\033[?25l"
	ShowCursor  = "\033[?25h"
)

// Box drawing sharp characters
const (
	TopLeft     = "┌"
	TopRight    = "┐"
	BottomLeft  = "└"
	BottomRight = "┘"
	Horizontal  = "─"
	Vertical    = "│"
	TeeLeft     = "├"
	TeeRight    = "┤"
	BlockFull   = "█"
	BlockEmpty  = "░"
	Arrow       = "▶"
)

const BoxWidth = 72

// CenterText centers text within a given width.
func CenterText(text string, width int) string {
	cleanLen := len(stripAnsi(text))
	if cleanLen >= width {
		return text
	}
	left := (width - cleanLen) / 2
	right := width - cleanLen - left
	return strings.Repeat(" ", left) + text + strings.Repeat(" ", right)
}

// Line produces a horizontal divider with sharp corners or tees.
func Line(left, fill, right string, width int) string {
	return left + strings.Repeat(fill, width-2) + right
}

// FormatSpeed converts bytes/sec into a clean readable string (e.g., 35.5 MB/s).
func FormatSpeed(bytesPerSec int64) string {
	mb := float64(bytesPerSec) / (1024 * 1024)
	if mb >= 1000 {
		return fmt.Sprintf("%.2f GB/s", mb/1024)
	}
	return fmt.Sprintf("%.1f MB/s", mb)
}

// FormatBytes converts bytes into a clean string (e.g., 2.45 GB).
func FormatBytes(bytes int64) string {
	mb := float64(bytes) / (1024 * 1024)
	if mb >= 1024 {
		return fmt.Sprintf("%.2f GB", mb/1024)
	}
	return fmt.Sprintf("%.1f MB", mb)
}

// ProgressBar generates a sharp high-contrast block progress bar.
func ProgressBar(percentage float64, width int) string {
	if percentage < 0 {
		percentage = 0
	}
	if percentage > 100 {
		percentage = 100
	}

	filled := int((percentage / 100.0) * float64(width))
	if filled > width {
		filled = width
	}
	empty := width - filled

	return BoldWhite + strings.Repeat(BlockFull, filled) + MutedGray + strings.Repeat(BlockEmpty, empty) + Reset
}

// RenderLiveDownload prints the live telemetry HUD for active downloads.
func RenderLiveDownload(s engine.ProgressSnapshot, pool *bond.AdapterPool, notification string) string {
	var b strings.Builder

	b.WriteString("\033[H") // Return cursor to top-left without clearing to avoid flicker
	b.WriteString(BoldWhite + Line(TopLeft, Horizontal, TopRight, BoxWidth) + "\n" + Reset)
	
	// Header
	header := fmt.Sprintf("%s TURBODL // HARDWARE SPEED BONDING ENGINE %s", BoldWhite, Reset)
	b.WriteString(fmt.Sprintf("%s%s %-70s%s%s\n", BoldWhite, Vertical, header, Vertical, Reset))
	
	statusTag := fmt.Sprintf("%s %s %s", InverseBold, strings.ToUpper(string(s.Status)), Reset)
	b.WriteString(fmt.Sprintf("%s%s  TARGET: %s%-45s%s %s%s%s\n", BoldWhite, Vertical, BoldWhite, truncate(s.Filename, 45), Reset, statusTag, Vertical, Reset))
	b.WriteString(BoldWhite + Line(TeeLeft, Horizontal, TeeRight, BoxWidth) + "\n" + Reset)

	// Speedometer (Hero Section)
	totalSpeedStr := FormatSpeed(s.TotalSpeedBps)
	heroLine := fmt.Sprintf("%s  AGGREGATED BANDWIDTH:  %s  %s  %s", DimWhite, InverseBold, totalSpeedStr, Reset)
	b.WriteString(fmt.Sprintf("%s%s %-79s%s%s\n", BoldWhite, Vertical, heroLine, Vertical, Reset))
	b.WriteString(BoldWhite + Line(TeeLeft, Horizontal, TeeRight, BoxWidth) + "\n" + Reset)

	// Adapter breakdown rows
	b.WriteString(fmt.Sprintf("%s%s  %s%-28s %-22s %-16s%s%s%s\n", BoldWhite, Vertical, MutedGray, "INTERFACE", "LOCAL IP", "SPEED", Reset, Vertical, Reset))
	
	adapters := pool.GetAll()
	for _, a := range adapters {
		speed := s.AdapterSpeeds[a.ID]
		speedStr := FormatSpeed(speed)
		tag := "[WIFI]"
		typeLabel := "PC WI-FI"
		if a.IsPhone {
			tag = "[PHONE]"
			typeLabel = "TETHER"
		}

		label := fmt.Sprintf("%-7s %-10s [%s]", tag, typeLabel, a.Name)
		ipStr := a.IP.String()

		// Speed bar for adapter
		meterWidth := 14
		meterPct := 0.0
		if s.TotalSpeedBps > 0 {
			meterPct = (float64(speed) / float64(s.TotalSpeedBps)) * 100
		}
		miniBar := ProgressBar(meterPct, meterWidth)

		row := fmt.Sprintf("  %-26s %-16s %s %8s", truncate(label, 26), ipStr, miniBar, speedStr)
		b.WriteString(fmt.Sprintf("%s%s %-70s%s%s\n", BoldWhite, Vertical, row, Vertical, Reset))
	}

	b.WriteString(BoldWhite + Line(TeeLeft, Horizontal, TeeRight, BoxWidth) + "\n" + Reset)

	// Progress Bar Section
	progBar := ProgressBar(s.Percentage, 46)
	b.WriteString(fmt.Sprintf("%s%s  PROGRESS: [%s] %5.1f%%%s%s\n", BoldWhite, Vertical, progBar, s.Percentage, Vertical, Reset))
	
	etaStr := "--:--"
	if s.ETASeconds > 0 {
		etaStr = (time.Duration(s.ETASeconds) * time.Second).String()
	}

	statLine := fmt.Sprintf("  TRANSFERRED: %s / %s      ETA: %-8s   CHUNKS: %d/%d",
		FormatBytes(s.Downloaded), FormatBytes(s.TotalSize), etaStr, s.DoneChunks, s.TotalChunks)
	b.WriteString(fmt.Sprintf("%s%s %-70s%s%s\n", BoldWhite, Vertical, statLine, Vertical, Reset))

	// Notification line if any (e.g. Phone connected)
	if notification != "" {
		b.WriteString(BoldWhite + Line(TeeLeft, Horizontal, TeeRight, BoxWidth) + "\n" + Reset)
		alertLine := fmt.Sprintf("  %s %s %s", InverseBold, notification, Reset)
		b.WriteString(fmt.Sprintf("%s%s %-79s%s%s\n", BoldWhite, Vertical, alertLine, Vertical, Reset))
	}

	b.WriteString(BoldWhite + Line(BottomLeft, Horizontal, BottomRight, BoxWidth) + "\n" + Reset)
	b.WriteString(fmt.Sprintf(" %s[CTRL+C] QUIT / DETACH    [P] PAUSE/RESUME    [HOT-PLUG ACTIVE]%s\n", MutedGray, Reset))

	return b.String()
}

// RenderMainMenu returns the sharp minimalist main menu screen.
func RenderMainMenu(adapters []*bond.Adapter, activeDownload bool) string {
	var b strings.Builder

	b.WriteString(ClearScreen)
	b.WriteString(BoldWhite + Line(TopLeft, Horizontal, TopRight, BoxWidth) + "\n" + Reset)
	
	title := fmt.Sprintf("%s  TURBODL // HARDWARE SPEED BONDING ENGINE v1.0.0  %s", BoldWhite, Reset)
	b.WriteString(fmt.Sprintf("%s%s %-78s%s%s\n", BoldWhite, Vertical, title, Vertical, Reset))
	
	statusLine := fmt.Sprintf("  %s HARDWARE BONDING %s  PHONE AUTO-DETECT: %s ACTIVE %s", 
		InverseBold, Reset, Inverse, Reset)
	b.WriteString(fmt.Sprintf("%s%s %-87s%s%s\n", BoldWhite, Vertical, statusLine, Vertical, Reset))
	b.WriteString(BoldWhite + Line(TeeLeft, Horizontal, TeeRight, BoxWidth) + "\n" + Reset)

	b.WriteString(fmt.Sprintf("%s%s                                                                      %s%s\n", BoldWhite, Vertical, Vertical, Reset))
	b.WriteString(fmt.Sprintf("%s%s   %s[1]%s START ACCELERATED DOWNLOAD (ENTER URL)                        %s%s\n", BoldWhite, Vertical, InverseBold, Reset, Vertical, Reset))
	b.WriteString(fmt.Sprintf("%s%s   %s[2]%s HARDWARE ADAPTER MONITOR (LIVE USB WATCHER)                   %s%s\n", BoldWhite, Vertical, InverseBold, Reset, Vertical, Reset))
	b.WriteString(fmt.Sprintf("%s%s   %s[3]%s QUICK INTERNET SPEED TEST PER ADAPTER                         %s%s\n", BoldWhite, Vertical, InverseBold, Reset, Vertical, Reset))
	b.WriteString(fmt.Sprintf("%s%s   %s[4]%s SETTINGS (THREADS, CHUNK SIZE, PHONE PROFILES)                %s%s\n", BoldWhite, Vertical, InverseBold, Reset, Vertical, Reset))
	b.WriteString(fmt.Sprintf("%s%s   %s[0]%s EXIT                                                           %s%s\n", BoldWhite, Vertical, InverseBold, Reset, Vertical, Reset))
	b.WriteString(fmt.Sprintf("%s%s                                                                      %s%s\n", BoldWhite, Vertical, Vertical, Reset))

	b.WriteString(BoldWhite + Line(TeeLeft, Horizontal, TeeRight, BoxWidth) + "\n" + Reset)

	// Detected interfaces overview
	b.WriteString(fmt.Sprintf("%s%s  %sDETECTED NETWORK HARDWARE:%s                                           %s%s\n", BoldWhite, Vertical, BoldWhite, Reset, Vertical, Reset))
	for _, a := range adapters {
		tag := "[PC WI-FI]"
		if a.IsPhone {
			tag = "[PHONE TETHER]"
		}
		adapterStr := fmt.Sprintf("   %-16s %-12s IP: %-16s %sONLINE%s", tag, a.Name, a.IP.String(), BoldWhite, Reset)
		b.WriteString(fmt.Sprintf("%s%s %-78s%s%s\n", BoldWhite, Vertical, adapterStr, Vertical, Reset))
	}

	b.WriteString(BoldWhite + Line(BottomLeft, Horizontal, BottomRight, BoxWidth) + "\n" + Reset)
	b.WriteString(fmt.Sprintf("\n %s%s SELECT OPTION %s ", InverseBold, Arrow, Reset) + " ")

	return b.String()
}

func truncate(str string, length int) string {
	if len(str) <= length {
		return str
	}
	if length <= 3 {
		return str[:length]
	}
	return str[:length-3] + "..."
}

func stripAnsi(str string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range str {
		if r == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
