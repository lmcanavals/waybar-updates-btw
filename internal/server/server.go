// Package server provides the D-Bus daemon server implementation and background update coordinators.
package server

import (
	"encoding/json"
	"log"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/lmcanavals/waybar-updates-btw/internal/aur"
	"github.com/lmcanavals/waybar-updates-btw/internal/pacman"
	"github.com/lmcanavals/waybar-updates-btw/internal/protocol"
)

// Server manages daemon state and handles D-Bus method calls.
type Server struct {
	conn *dbus.Conn

	dataMutex     sync.RWMutex
	pacmanUpdates []string
	aurUpdates    []string
	version       int64
	lastUpdated   time.Time

	triggerCheck     chan struct{}
	isCheckingPacman atomic.Bool
	isCheckingAur    atomic.Bool
}

// New creates and initializes a new Server.
func New(conn *dbus.Conn) *Server {
	return &Server{
		conn:          conn,
		version:       0,
		lastUpdated:   time.Now(),
		pacmanUpdates: []string{},
		aurUpdates:    []string{},
		triggerCheck:  make(chan struct{}, 1),
	}
}

// GetUpdates takes the client's last known version.
// If they match, it returns a lightweight "changed: false" response.
func (s *Server) GetUpdates(clientVersion int64) (string, *dbus.Error) {
	s.dataMutex.RLock()
	defer s.dataMutex.RUnlock()

	log.Printf("Received GetInfo call. Client Ver: %d | Server Ver: %d", clientVersion, s.version)

	resp := protocol.ResponseData{
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

// CheckNow immediately triggers a full check asynchronously, resetting periodic timers.
func (s *Server) CheckNow() *dbus.Error {
	if s.isCheckingPacman.Load() || s.isCheckingAur.Load() {
		log.Println("CheckNow ignored: update check already in progress")
		return nil
	}

	select {
	case s.triggerCheck <- struct{}{}:
		log.Println("CheckNow triggered")
	default:
		log.Println("CheckNow ignored: check already queued")
	}
	return nil
}

// updateState updates the internal lists and increments version if data changed.
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
		s.version++
		s.lastUpdated = time.Now()
		log.Printf("State updated by %s. New Version: %d", source, s.version)

		_ = s.conn.Emit(protocol.ObjectPath, protocol.InterfaceName+"."+protocol.SignalInfoUpdated, s.version)
	}
}

// doFullCheck runs a full check on both pacman and aur concurrently, updates state, and emits InfoUpdated.
func (s *Server) doFullCheck() {
	if !s.isCheckingPacman.CompareAndSwap(false, true) {
		log.Println("Pacman check already in progress, skipping full check")
		return
	}
	defer s.isCheckingPacman.Store(false)

	if !s.isCheckingAur.CompareAndSwap(false, true) {
		log.Println("AUR check already in progress, skipping full check")
		return
	}
	defer s.isCheckingAur.Store(false)

	var wg sync.WaitGroup
	var pUpdates []string
	var pOk bool
	var aUpdates []string
	var aOk bool

	wg.Add(2)
	go func() {
		defer wg.Done()
		updates, ok, err := pacman.Check(true)
		if err != nil {
			log.Printf("Pacman full check error: %v", err)
			return
		}
		pUpdates, pOk = updates, ok
	}()
	go func() {
		defer wg.Done()
		updates, ok, err := aur.Check()
		if err != nil {
			log.Printf("AUR check error: %v", err)
			return
		}
		aUpdates, aOk = updates, ok
	}()
	wg.Wait()

	s.dataMutex.Lock()
	defer s.dataMutex.Unlock()

	if pOk {
		s.pacmanUpdates = pUpdates
	}
	if aOk {
		s.aurUpdates = aUpdates
	}
	s.version++
	s.lastUpdated = time.Now()
	log.Printf("Full check completed. New Version: %d (Pacman: %d, AUR: %d)", s.version, len(s.pacmanUpdates), len(s.aurUpdates))

	_ = s.conn.Emit(protocol.ObjectPath, protocol.InterfaceName+"."+protocol.SignalInfoUpdated, s.version)
}

// Run executes the main background coordinator loop.
func (s *Server) Run(pacmanInterval, aurInterval time.Duration, updateOnIter int) {
	var iter atomic.Int64

	// Initial check on startup
	s.doFullCheck()

	pacmanTimer := time.NewTimer(pacmanInterval)
	aurTimer := time.NewTimer(aurInterval)
	defer pacmanTimer.Stop()
	defer aurTimer.Stop()

	for {
		select {
		case <-s.triggerCheck:
			log.Println("Manual CheckNow triggered: running full check and resetting timers...")
			s.doFullCheck()
			resetTimer(pacmanTimer, pacmanInterval)
			resetTimer(aurTimer, aurInterval)
			iter.Store(0)

		case <-pacmanTimer.C:
			currentIter := iter.Add(1)
			fullSync := currentIter >= int64(updateOnIter)

			go func(full bool) {
				if !s.isCheckingPacman.CompareAndSwap(false, true) {
					return
				}
				defer s.isCheckingPacman.Store(false)

				updates, ok, err := pacman.Check(full)
				if err != nil {
					log.Printf("Pacman check error: %v", err)
					return
				}
				if ok {
					s.updateState("pacman", updates)
					if full {
						iter.Store(0)
					}
				}
			}(fullSync)

			pacmanTimer.Reset(pacmanInterval)

		case <-aurTimer.C:
			go func() {
				if !s.isCheckingAur.CompareAndSwap(false, true) {
					return
				}
				defer s.isCheckingAur.Store(false)

				updates, ok, err := aur.Check()
				if err != nil {
					log.Printf("AUR check error: %v", err)
					return
				}
				if ok {
					s.updateState("aur", updates)
				}
			}()

			aurTimer.Reset(aurInterval)
		}
	}
}

func resetTimer(t *time.Timer, d time.Duration) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
	t.Reset(d)
}
