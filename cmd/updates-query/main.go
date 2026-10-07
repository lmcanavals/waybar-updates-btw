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
	"github.com/lmcanavals/waybar-updates-btw/internal/format"
	"github.com/lmcanavals/waybar-updates-btw/internal/protocol"
)

const (
	formatWaybar = "waybar"
	formatJSON   = "json"
)

func main() {
	var (
		interval                                                 int
		rawOutput, noColor, checkNow, once                       bool
		outputFormat                                             string
		colorMajor, colorMinor, colorPatch, colorPre, colorOther string
	)
	flag.IntVar(&interval, "interval", 120, "Set the interval between D-Bus queries in seconds.")
	flag.BoolVar(&checkNow, "check-now", false, "Trigger an immediate full check on the updates-fetch daemon and exit.")
	flag.StringVar(&outputFormat, "format", formatWaybar, "Output format: 'waybar' (Pango-formatted for Waybar) or 'json' (generic structured data).")
	flag.BoolVar(&once, "once", false, "Print the current state once and exit instead of streaming changes.")
	flag.BoolVar(&rawOutput, "raw-output", false, "Disables formatting tooltip text into columns (waybar format only).")
	flag.BoolVar(&noColor, "no-color", false, "Disables coloring packages by version category (waybar format only).")
	flag.StringVar(&colorMajor, "color-major", "f7768e", "Color for major version update (waybar format only).")
	flag.StringVar(&colorMinor, "color-minor", "ff9e64", "Color for minor version update (waybar format only).")
	flag.StringVar(&colorPatch, "color-patch", "e0af68", "Color for patch update (waybar format only).")
	flag.StringVar(&colorPre, "color-pre", "9ece6a", "Color for pre update (waybar format only).")
	flag.StringVar(&colorOther, "color-other", "7dcfff", "Color for other update (waybar format only).")
	flag.Parse()

	if outputFormat != formatWaybar && outputFormat != formatJSON {
		fmt.Fprintf(os.Stderr, "invalid -format %q: must be %q or %q\n", outputFormat, formatWaybar, formatJSON)
		os.Exit(2)
	}

	colors := []string{colorMajor, colorMinor, colorPatch, colorPre, colorOther}

	conn, err := dbus.SessionBus()
	if err != nil {
		log.Fatalf("Failed to connect to session bus: %v", err)
	}
	defer func() { _ = conn.Close() }()

	obj := conn.Object(protocol.BusName, dbus.ObjectPath(protocol.ObjectPath))

	if checkNow {
		call := obj.Call(protocol.InterfaceName+".CheckNow", 0)
		if call.Err != nil {
			log.Fatalf("Failed to trigger CheckNow: %v", call.Err)
		}
		fmt.Println("CheckNow triggered successfully")
		return
	}

	encoder := json.NewEncoder(os.Stdout)

	printWaybar := func(updates []string) {
		result := format.UIResult{
			Text:    "",
			Tooltip: "All packages are up to date",
			Class:   "updated",
			Alt:     "updated",
		}

		if len(updates) > 0 {
			formattedUpdates := make([]string, len(updates))
			copy(formattedUpdates, updates)

			if !rawOutput || !noColor {
				format.AddFormat(formattedUpdates, colors, rawOutput, noColor)
			}

			result.Text = fmt.Sprintf("󰮯 %d", len(updates))
			result.Tooltip = strings.Join(formattedUpdates, "\n")
			result.Class = "has-updates"
			result.Alt = "has-updates"
		}

		if err := encoder.Encode(result); err != nil {
			log.Printf("Error encoding JSON: %v", err)
		}
	}

	printStatus := func(resp protocol.ResponseData) {
		if outputFormat == formatJSON {
			if err := encoder.Encode(format.BuildJSON(resp)); err != nil {
				log.Printf("Error encoding JSON: %v", err)
			}
			return
		}
		printWaybar(resp.Updates)
	}

	currentVersion := int64(-1)

	getUpdates := func() (protocol.ResponseData, error) {
		var resp protocol.ResponseData
		call := obj.Call(protocol.InterfaceName+".GetUpdates", 0, currentVersion)
		if call.Err != nil {
			return resp, call.Err
		}
		var jsonResult string
		if err := call.Store(&jsonResult); err != nil {
			return resp, err
		}
		if err := json.Unmarshal([]byte(jsonResult), &resp); err != nil {
			return resp, err
		}
		return resp, nil
	}

	if once {
		resp, err := getUpdates()
		if err != nil {
			log.Fatalf("Failed to get updates: %v", err)
		}
		printStatus(resp)
		return
	}

	// Subscribe to Signals
	matchRule := fmt.Sprintf("type='signal',interface='%s',member='%s',path='%s'",
		protocol.InterfaceName, protocol.SignalInfoUpdated, protocol.ObjectPath)
	call := conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, matchRule)
	if call.Err != nil {
		log.Fatalf("Failed to add D-Bus match rule: %v", call.Err)
	}
	signalChan := make(chan *dbus.Signal, 10)
	conn.Signal(signalChan)

	fetchData := func() {
		resp, err := getUpdates()
		if err != nil {
			// Daemon may be temporarily unavailable; retry on next tick or signal.
			return
		}
		if resp.Changed {
			currentVersion = resp.Version
			printStatus(resp)
		}
	}

	// Initial Fetch
	fetchData()

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	for {
		select {
		case <-ticker.C:
			fetchData()
		case <-signalChan:
			fetchData()
		case <-interrupt:
			return
		}
	}
}
