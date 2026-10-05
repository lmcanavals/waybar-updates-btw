package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/lmcanavals/waybar-updates-btw/internal/protocol"
	"github.com/lmcanavals/waybar-updates-btw/internal/server"
)

func main() {
	conn, err := dbus.SessionBus()
	if err != nil {
		log.Fatalf("Failed to connect to session bus: %v", err)
	}
	defer func() { _ = conn.Close() }()

	reply, err := conn.RequestName(protocol.BusName, dbus.NameFlagReplaceExisting)
	if err != nil {
		log.Fatalf("Failed to request D-Bus name: %v", err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		fmt.Fprintln(os.Stderr, "Name already taken")
		os.Exit(1)
	}

	srv := server.New(conn)

	err = conn.Export(srv, protocol.ObjectPath, protocol.InterfaceName)
	if err != nil {
		log.Fatalf("Couldn't export D-Bus interface: %v", err)
	}

	node := &introspect.Node{
		Name: string(protocol.ObjectPath),
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			{
				Name:    protocol.InterfaceName,
				Methods: introspect.Methods(srv),
				Signals: []introspect.Signal{
					{
						Name: protocol.SignalInfoUpdated,
						Args: []introspect.Arg{
							{Name: "version", Type: "x"},
						},
					},
				},
			},
		},
	}
	err = conn.Export(introspect.NewIntrospectable(node), protocol.ObjectPath, "org.freedesktop.DBus.Introspectable")
	if err != nil {
		log.Printf("Couldn't export introspectable: %v", err)
	}
	log.Printf("Service %s running...", protocol.BusName)

	srv.Run(1*time.Minute, 5*time.Minute, 10)
}
