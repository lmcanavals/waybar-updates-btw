package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// --- D-BUS CONSTANTS ---
const (
	BusName       = "org.lmcs.DBus.UpdatesBtw"
	ObjectPath    = "/org/lmcs/DBus/UpdatesBtw/GetUpdates"
	InterfaceName = "org.lmcs.DBus.UpdatesBtw.UpdatesInterface"
)

// --- OUTPUT STRUCTURE (Matches your old app's output) ---
type uIResult struct {
	Text    string `json:"text"`
	Tooltip string `json:"tooltip"`
	Class   string `json:"class"`
	Alt     string `json:"alt"`
}

// --- D-BUS PAYLOAD STRUCTURE (Matches server) ---
type responseData struct {
	Changed   bool     `json:"changed"`
	Version   int64    `json:"version"`
	Updates   []string `json:"updates,omitempty"`
	Count     int      `json:"count"`
	Timestamp string   `json:"timestamp"`
}

// Global state
var (
	currentVersion int64    = -1
	currentUpdates []string // Cache the raw updates to re-print if needed
)

func main() {
	// --- FLAGS (Ported from old app) ---
	var (
		interval                                                 int
		rawOutput, noColor                                       bool
		colorMajor, colorMinor, colorPatch, colorPre, colorOther string
	)
	flag.IntVar(&interval, "interval", 120, "Set the interval between D-Bus queries in seconds.") // Default 2 mins
	flag.BoolVar(&rawOutput, "raw-output", false, "Disables formatting tooltip text into columns.")
	flag.BoolVar(&noColor, "no-color", false, "Disables coloring packages by version category.")
	flag.StringVar(&colorMajor, "color-major", "f7768e", "Color for major version update.")
	flag.StringVar(&colorMinor, "color-minor", "ff9e64", "Color for minor version update.")
	flag.StringVar(&colorPatch, "color-patch", "e0af68", "Color for patch update.")
	flag.StringVar(&colorPre, "color-pre", "9ece6a", "Color for pre update.")
	flag.StringVar(&colorOther, "color-other", "7dcfff", "Color for other update.")
	flag.Parse()

	colors := []string{colorMajor, colorMinor, colorPatch, colorPre, colorOther}

	// --- DBUS SETUP ---
	conn, err := dbus.SessionBus()
	if err != nil {
		log.Fatalf("Failed to connect to session bus: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Subscribe to Signals
	matchRule := fmt.Sprintf("type='signal',interface='%s',member='InfoUpdated',path='%s'", InterfaceName, ObjectPath)
	call := conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, matchRule)
	if call.Err != nil {
		log.Fatalf("Failed to add D-Bus match rule: %v", call.Err)
	}
	signalChan := make(chan *dbus.Signal, 10)
	conn.Signal(signalChan)

	obj := conn.Object(BusName, dbus.ObjectPath(ObjectPath))

	// Helper to handle formatting and printing JSON to stdout
	printStatus := func(updates []string) {
		result := uIResult{
			Text:    "0",
			Tooltip: "All packages are up to date",
			Class:   "updated",
			Alt:     "updated",
		}

		if len(updates) > 0 {
			// Work on a copy so we don't mutate the cached raw data
			formattedUpdates := make([]string, len(updates))
			copy(formattedUpdates, updates)

			if !rawOutput || !noColor {
				addFormat(formattedUpdates, colors, rawOutput, noColor)
			}

			result.Text = fmt.Sprintf("%d", len(updates))
			result.Tooltip = strings.Join(formattedUpdates, "\n")
			result.Class = "has-updates"
			result.Alt = "has-updates"
		}

		encoder := json.NewEncoder(os.Stdout)
		// Ensure single line JSON for consumption by bars (waybar/polybar etc)
		// encoder.SetIndent("", "")
		if err := encoder.Encode(result); err != nil {
			log.Printf("Error encoding JSON: %v", err)
		}
	}

	// Fetch logic
	fetchData := func() {
		call := obj.Call(InterfaceName+".GetUpdates", 0, currentVersion)
		if call.Err != nil {
			// If server is down, we might want to print an error state or just wait
			// log.Printf("Failed to call GetUpdates: %v", call.Err)
			return
		}

		var jsonResult string
		if err := call.Store(&jsonResult); err != nil {
			return
		}

		var resp responseData
		if err := json.Unmarshal([]byte(jsonResult), &resp); err != nil {
			return
		}

		if resp.Changed {
			currentVersion = resp.Version
			currentUpdates = resp.Updates
			printStatus(currentUpdates)
		}
	}

	// Initial Fetch
	fetchData()

	// --- MAIN LOOP ---
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	for {
		select {
		case <-ticker.C:
			fetchData()
		case <-signalChan:
			// Signal received, fetch immediately
			fetchData()
		case <-interrupt:
			return
		}
	}
}

// --- FORMATTING LOGIC (Ported from old app) ---

func addFormat(updates, colors []string, rawOutput, noColor bool) {
	allParts := make([][]string, len(updates))
	maxNameLen := 0
	maxVersionLen := 0
	var formatStr strings.Builder

	// 1. Parse and find max lengths
	for i, line := range updates {
		allParts[i] = strings.Fields(line)
		// Expected format: "package 1.0 -> 2.0" OR "aur/package 1.0 -> 2.0"
		// Fields should be >= 4 to contain name, v1, ->, v2
		if len(allParts[i]) < 4 {
			continue
		}
		maxNameLen = max(maxNameLen, len(allParts[i][0]))
		maxVersionLen = max(maxVersionLen, len(allParts[i][1]))
	}

	// 2. Format strings
	for i, part := range allParts {
		formatStr.Reset()
		if len(part) < 4 {
			continue
		}

		// The old version is at index 1, new version is at index 3
		oldVer := part[1]
		newVer := part[3]

		fmt.Fprint(&formatStr, "<span font-family='monospace'")
		if !noColor {
			category := parseVersion(oldVer, newVer)
			// Safety check for index
			if category >= 0 && category < len(colors) {
				fmt.Fprintf(&formatStr, " color='#%s'", colors[category])
			}
		}

		if rawOutput {
			fmt.Fprintf(&formatStr, ">%%s %%s -> %%s</span>")
		} else {
			// Pad name and old version for alignment
			fmt.Fprintf(&formatStr, ">%%-%ds %%-%ds -> %%s</span>", maxNameLen, maxVersionLen)
		}

		updates[i] = fmt.Sprintf(formatStr.String(), part[0], oldVer, newVer)
	}
}

func parseVersion(oldVersion, newVersion string) int {
	dotCounter := 0
	maxLen := max(len(oldVersion), len(newVersion))

	// Simple logic: count dots until the first difference char
	for i := range maxLen {
		// Handle out of bounds if strings are diff lengths
		var cOld, cNew byte
		if i < len(oldVersion) {
			cOld = oldVersion[i]
		}
		if i < len(newVersion) {
			cNew = newVersion[i]
		}

		if cNew == '.' || cNew == '-' {
			dotCounter++
		}

		if cNew != cOld {
			break
		}
	}

	// Map dot count to color index (major, minor, patch, etc)
	// Example: 0 diffs before 1st dot = Major (index 0)
	// Note: You might want to cap this at len(colors)-1
	if dotCounter > 4 {
		return 4 // colorOther
	}
	return dotCounter
}
