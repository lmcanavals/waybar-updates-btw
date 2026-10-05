// Package protocol defines shared D-Bus constants, signal names, and response schemas.
package protocol

const (
	BusName           = "org.lmcs.DBus.UpdatesBtw"
	ObjectPath        = "/org/lmcs/DBus/UpdatesBtw/GetUpdates"
	InterfaceName     = "org.lmcs.DBus.UpdatesBtw.UpdatesInterface"
	SignalInfoUpdated = "InfoUpdated"
)

// ResponseData is the D-Bus payload sent to clients.
type ResponseData struct {
	Changed   bool     `json:"changed"`           // True if data is fresh
	Version   int64    `json:"version"`           // The current server version
	Updates   []string `json:"updates,omitempty"` // The list (omitted if changed==false)
	Count     int      `json:"count"`             // Total count
	Timestamp string   `json:"timestamp"`         // RFC3339 timestamp
}
