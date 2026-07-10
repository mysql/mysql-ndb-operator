// Copyright (c) 2026, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"
)

func runServer(listenAddress string) {
	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", listenAddress, err)
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Fatalf("failed to accept connection: %v", err)
		}
		_, _ = io.WriteString(conn, "ok\n")
		_ = conn.Close()
	}
}

func runClient(address string, timeout time.Duration) {
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		log.Fatalf("failed to connect to %s: %v", address, err)
	}
	defer conn.Close()

	if _, err = io.ReadAll(conn); err != nil {
		log.Fatalf("failed to read from %s: %v", address, err)
	}
}

func main() {
	mode := flag.String("mode", "", "server or client")
	listenAddress := flag.String("listen", ":8080", "server listen address")
	address := flag.String("address", "", "client target address")
	timeout := flag.Duration("timeout", 3*time.Second, "client connect timeout")
	flag.Parse()

	switch *mode {
	case "server":
		runServer(*listenAddress)
	case "client":
		if *address == "" {
			log.Fatal("address is required in client mode")
		}
		runClient(*address, *timeout)
	default:
		fmt.Fprintf(os.Stderr, "unsupported mode %q\n", *mode)
		os.Exit(2)
	}
}
