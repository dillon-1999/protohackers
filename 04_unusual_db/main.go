package main

import (
	"fmt"
	"net"
	"os"
	"strings"
)

var (
	PORT        = 8899
	VERSION     = "1.0.0"
	SERVER_NAME = "Dillons KV Store"
	KW_VERSION  = "version"
)

func version() string {
	return fmt.Sprintf("%s %s", SERVER_NAME, VERSION)
}

func respond(ln *net.UDPConn, remoteAddr *net.UDPAddr, message string) error {
	_, err := ln.WriteToUDP([]byte(message), remoteAddr)
	return err
}

func main() {
	localAddr, err := net.ResolveUDPAddr("udp", ":8899")
	ln, err := net.ListenUDP("udp", localAddr)
	if err != nil {
		fmt.Println("Failed to listen to address:", err)
		os.Exit(1)
	}
	defer ln.Close()

	kv := make(map[string]string)

	// protocal states the message must be < 1000 bytes
	buf := make([]byte, 1000)

	// TODO: see if we can just work in bytes and not convert to strings
	for {
		size, remoteAddr, err := ln.ReadFromUDP(buf)
		if err != nil {
			fmt.Println("Failed to read from udp socket:", err)
			continue
		}
		message := string(buf[:size])

		// keyword responses
		if message == KW_VERSION {
			respond(ln, remoteAddr, fmt.Sprintf("version=%s", version()))
			continue
		}

		// split on the first '='
		parts := strings.SplitN(message, "=", 2)
		if len(parts) == 1 { // no =, must be a request
			v, ok := kv[parts[0]]
			if !ok {
				fmt.Println("Value does not exist at:", parts[0])
				continue
			}
			resp := fmt.Sprintf("%s=%s", parts[0], v)
			respond(ln, remoteAddr, resp)
		} else { // must be an insert
			k, v := parts[0], parts[1]
			kv[k] = v
			// no response necessary
		}
	}
}
