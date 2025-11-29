package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	BusName       = "org.lmcs.DBus.UpdatesBtw"
	ObjectPath    = "/org/lmcs/DBus/UpdatesBtw/GetUpdates"
	InterfaceName = "org.lmcs.DBus.UpdatesBtw.UpdatesInterface"
)

// ResponseData is the payload sent to the client
type ResponseData struct {
	Changed   bool     `json:"changed"`           // True if data is fresh
	Version   int64    `json:"version"`           // The current server version
	Updates   []string `json:"updates,omitempty"` // The list (omitted if changed==false)
	Count     int      `json:"count"`             // Total count
	Timestamp string   `json:"timestamp"`
}

// Server holds the state
type Server struct {
	conn *dbus.Conn

	dataMutex     sync.RWMutex
	pacmanUpdates []string
	aurUpdates    []string
	version       int64
	lastUpdated   time.Time
}

type AurPackage struct {
	Name    string `json:"Name"`
	Version string `json:"Version"`
}
type AurResponse struct {
	Results []AurPackage `json:"results"`
}

// GetUpdates takes the client's last known version.
// If they match, we return a lightweight "no change" response.
func (s *Server) GetUpdates(clientVersion int64) (string, *dbus.Error) {
	s.dataMutex.RLock()
	defer s.dataMutex.RUnlock()

	log.Printf("Received GetInfo call. Client Ver: %d | Server Ver: %d", clientVersion, s.version)

	resp := ResponseData{
		Version:   s.version,
		Timestamp: s.lastUpdated.Format(time.RFC3339),
		Count:     len(s.pacmanUpdates) + len(s.aurUpdates),
	}

	if clientVersion == s.version {
		resp.Changed = false
	} else {
		resp.Changed = true
		resp.Updates = append(s.pacmanUpdates, s.aurUpdates...)
	}

	jsonData, err := json.Marshal(resp)
	if err != nil {
		return "", dbus.NewError("org.lmcs.Error.MarshalFailed", []any{err.Error()})
	}

	return string(jsonData), nil
}

// updateState updates the internal lists and increments version if data changed
func (s *Server) updateState(source string, newUpdates []string) {
	s.dataMutex.Lock()
	defer s.dataMutex.Unlock()

	var changed bool

	switch source {
	case "pacman":
		if !slices.Equal(s.pacmanUpdates, newUpdates) {
			s.pacmanUpdates = newUpdates
			changed = true
		}
	case "aur":
		if !slices.Equal(s.aurUpdates, newUpdates) {
			s.aurUpdates = newUpdates
			changed = true
		}
	}

	if changed {
		s.version++ // Increment version counter
		s.lastUpdated = time.Now()
		log.Printf("State updated by %s. New Version: %d", source, s.version)

		_ = s.conn.Emit(ObjectPath, InterfaceName+".InfoUpdated", s.version)
	}
}

func checkUpdates(chUpdates chan<- []string, updateOnIter int, intervalDuration time.Duration) {
	iter := updateOnIter

	for {
		var args []string
		runCmd := false

		if iter == updateOnIter {
			// Full sync: checkupdates (no flags)
			// We must run this to ensure DB is fresh
			args = []string{"checkupdates", "--nocolor"}
			iter = 0
			runCmd = true
		} else {
			// Fast check: checkupdates --nosync --change
			// If returns empty, it means "no change since last checkupdates run"
			args = []string{"checkupdates", "--nosync", "--change", "--nocolor"}
			runCmd = true
		}
		iter++

		if runCmd {
			cmd := exec.Command(args[0], args[1:]...)
			output, err := cmd.Output()
			outputStr := strings.TrimSpace(string(output))

			if err != nil {
				if exiterr, ok := err.(*exec.ExitError); ok && exiterr.ExitCode() == 2 {
					// Exit code 2 = No updates available
					chUpdates <- []string{}
				} else {
					// Real error (e.g. DB lock), do nothing or log
					log.Printf("checkupdates error: %v", err)
				}
			} else {
				// Success (Exit Code 0)
				if outputStr == "" {
					// If we used --change and got empty output, it means NO CHANGE.
					// We do NOT send anything to the channel, preserving the server's current state.
					if len(args) <= 2 {
						// If this was a full run (no --change) and output is empty,
						// it actually means 0 updates.
						chUpdates <- []string{}
					}
				} else {
					// We have output, so the list is valid and potentially new
					lines := strings.Split(outputStr, "\n")
					chUpdates <- lines
				}
			}
		}

		time.Sleep(intervalDuration)
	}
}

func checkAurUpdates(chUpdates chan<- []string, intervalDuration time.Duration) {
	// Initial check
	doAurCheck(chUpdates)

	ticker := time.NewTicker(intervalDuration)
	defer ticker.Stop()

	for range ticker.C {
		doAurCheck(chUpdates)
	}
}

func doAurCheck(chUpdates chan<- []string) {
	output, err := exec.Command("pacman", "-Qm").Output()
	if err != nil {
		return
	}

	localPackages := make(map[string]string)
	for line := range strings.SplitSeq(strings.TrimSpace(string(output)), "\n") {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			localPackages[parts[0]] = parts[1]
		}
	}

	if len(localPackages) == 0 {
		chUpdates <- []string{}
		return
	}

	packageNames := make([]string, 0, len(localPackages))
	for name := range localPackages {
		packageNames = append(packageNames, name)
	}

	aurPackages, err := queryAurAPI(packageNames)
	if err != nil {
		log.Printf("AUR API Error: %v", err)
		return
	}

	var updates []string

	for _, aurPkg := range aurPackages {
		localVer, ok := localPackages[aurPkg.Name]
		if ok && aurPkg.Version != localVer {
			updates = append(updates, fmt.Sprintf("aur/%s %s -> %s", aurPkg.Name, localVer, aurPkg.Version))
		}
	}

	chUpdates <- updates
}

func queryAurAPI(packageNames []string) ([]AurPackage, error) {
	if len(packageNames) == 0 {
		return nil, nil
	}
	u, _ := url.Parse("https://aur.archlinux.org/rpc/")
	q := u.Query()
	q.Set("v", "5")
	q.Set("type", "info")
	for _, name := range packageNames {
		q.Add("arg[]", name)
	}
	u.RawQuery = q.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var aurResponse AurResponse
	err = json.Unmarshal(body, &aurResponse)
	if err != nil {
		log.Printf("Couldn't unmarshal %v\n", err)
	}
	return aurResponse.Results, nil
}

func main() {
	conn, err := dbus.SessionBus()
	if err != nil {
		panic(err)
	}

	reply, err := conn.RequestName(BusName, dbus.NameFlagReplaceExisting)
	if err != nil {
		panic(err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		fmt.Fprintln(os.Stderr, "Name already taken")
		os.Exit(1)
	}

	server := &Server{
		conn:          conn,
		version:       1,
		lastUpdated:   time.Now(),
		pacmanUpdates: []string{},
		aurUpdates:    []string{},
	}

	err = conn.Export(server, ObjectPath, InterfaceName)
	if err != nil {
		log.Printf("Couldn't export error: %v", err)
		os.Exit(1)
	}
	log.Printf("Service %s running...", BusName)

	chArch := make(chan []string)
	chAur := make(chan []string)

	go checkUpdates(chArch, 10, 1*time.Minute)
	go checkAurUpdates(chAur, 5*time.Minute)

	for {
		select {
		case pUpdates := <-chArch:
			server.updateState("pacman", pUpdates)
		case aUpdates := <-chAur:
			server.updateState("aur", aUpdates)
		}
	}
}
