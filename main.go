package main

import (
	"flag"
	"log"
	"os"
	"os/signal"

	"golang.org/x/sys/unix"
)

var (
	interfaceName = flag.String("interface", "", "Interface to bind to")
	mode          = flag.String("mode", "client", "Mode (client or server)")
	address       = flag.String("address", "", "Address to bind to (server) or connect to (client)")
	allowedIP     = flag.String("allow", "127.0.0.1", "IP address allowed to connect (server mode only)")
)

func main() {
	flag.Parse()

	if *interfaceName == "" {
		log.Fatal("Interface name must be specified")
	}

	if *mode != "client" && *mode != "server" {
		log.Fatal("Mode must be 'client' or 'server'")
	}

	if *address == "" {
		log.Fatal("Address must be specified")
	}

	// Initialize discovery and session handlers
	discoveryHandler, err := NewDiscoveryHandler(*interfaceName, *mode == "server")
	if err != nil {
		log.Fatalf("Failed to initialize discovery handler: %v", err)
	}
	defer discoveryHandler.Close()

	sessionHandler, err := NewSessionHandler(*interfaceName, *mode == "server")
	if err != nil {
		log.Fatalf("Failed to initialize session handler: %v", err)
	}
	defer sessionHandler.Close()

	// Initialize proxy
	proxy, err := NewProxy(*mode == "server", *address, *allowedIP, discoveryHandler, sessionHandler)
	if err != nil {
		log.Fatalf("Failed to initialize proxy: %v", err)
	}
	defer proxy.Close()

	// Setup signal handling for graceful shutdown
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, unix.SIGINT, unix.SIGTERM)

	log.Printf("PPPoE proxy started in %s mode on interface %s", *mode, *interfaceName)
	if *mode == "server" {
		log.Printf("Listening on %s", *address)
	} else {
		log.Printf("Connecting to %s", *address)
	}

	// Wait for termination signal
	<-signalCh
	log.Println("Shutting down...")
}
