package main

// STATUS: Passed
import (
	"fmt"
	"io"
	"net"
	"os"
)

var (
	PORT = "8899"
)

func main() {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%s", PORT))
	if err != nil {
		fmt.Println("Listen: ", err.Error())
		os.Exit(1)
	}

	fmt.Printf("Listening on port: %s\n", PORT)
	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("Accept: ", err.Error())
			os.Exit(1)
		}
		fmt.Println("connection from: ", conn.RemoteAddr())
		go handle(conn)
	}
}

func handle(conn net.Conn) {
	defer func() {
		fmt.Println("Closing Connection: ", conn.RemoteAddr())
		conn.Close()
	}()

	for {
		buf := make([]byte, 65535)
		size, err := conn.Read(buf)
		if err != nil {
			if err == io.EOF {
				fmt.Println("Received EOF")
				return
			}
			fmt.Printf("Failed to read bytes from client %s: %s", conn.RemoteAddr(), err.Error())
			return
		}
		conn.Write(buf[:size])
	}
}
