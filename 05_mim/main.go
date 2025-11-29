package main

// STATUS: PASSED
import (
	"bufio"
	"fmt"
	"net"
	"os"
	"regexp"
)

var (
	LOCAL_PORT         = 8899
	PROXY_HOST         = "chat.protohackers.com"
	PROXY_PORT         = 16963
	TONY_ADDRESS       = "7YWHMfk9JZe0LM0g1ZauHuiSxhI"
	ADDRESS_RE_PATTERN = `(^|\s)(7[[:alnum:]]{25,34})(\s|$)`
)

func main() {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", LOCAL_PORT))
	if err != nil {
		fmt.Println("Failed to start server: ", err)
		os.Exit(1)
	}
	defer ln.Close()
	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("Failed to accept connection from : ", conn.RemoteAddr())
			continue
		}

		go handle(conn)
	}
}

// This function works, but is a bit of a nightmare..
// TODO: clean this up
func replaceToken(data []byte) []byte {
	re := regexp.MustCompile(`7[[:alnum:]]{25,34}`)
	var valid [][2]int
	matches := re.FindAllIndex(data, -1)

	for _, match := range matches {
		first, last := match[0], match[1]
		points := [2]int{-1, -1}

		if first == 0 || data[first-1] == ' ' {
			points[0] = first
		}
		if data[last] == ' ' || data[last] == '\n' {
			points[1] = last - 1
		}

		if points[0] != -1 && points[1] != -1 {
			valid = append(valid, points)
		}
	}
	if len(valid) > 0 {
		pieces := [][]byte{}

		i := 0
		for _, v := range valid {
			x := data[i:v[0]]
			pieces = append(pieces, x)
			i = v[1] + 1
		}
		pieces = append(pieces, data[i:])
		plen := len(pieces)
		newData := []byte{}
		for pi, piece := range pieces {
			// fmt.Printf("%d '%s'\n", pi, string(piece))
			newData = append(newData, piece...)
			if pi < plen-1 {
				newData = append(newData, []byte(TONY_ADDRESS)...)
			}
		}
		return newData
	}

	return data
}

type ConnectionType int

const (
	ClientConnection ConnectionType = iota
	ServerConnection
)

type Message struct {
	Payload    []byte
	SenderType ConnectionType
}

func handle(conn net.Conn) {
	// resolve IP from fqdn
	remoteAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", PROXY_HOST, PROXY_PORT))
	if err != nil {
		fmt.Println("Failed to resolve addr:", err)
		conn.Close() // cleanup connection
		return
	}
	addr := net.TCPAddr{IP: remoteAddr.IP, Port: PROXY_PORT}

	// create a relay for sending/receiving from the chat server
	relay, err := net.DialTCP("tcp", nil, &addr)
	if err != nil {
		fmt.Println("Failed to ")
		conn.Close() // cleanup connection
		return
	}

	/*
		I need to:
		1. listen to and forward events from the client
			1.a clean up client connection on disconnect
		2. listen to and forward events from the server
			2.a clean up server connections on disconnect
		3. when one side of the connection closes, close the corresponding connection
		4. replace BogusCoin tokens with Tonys
	*/
	messageChan := make(chan Message)
	closeChan := make(chan struct{})

	go stream(conn, ClientConnection, messageChan, closeChan)
	go stream(relay, ServerConnection, messageChan, closeChan)

	for {
		select {
		case m, _ := <-messageChan:
			fmt.Println(string(m.Payload))
			if m.SenderType == ClientConnection {
				relay.Write(m.Payload)
			} else {
				conn.Write(m.Payload)
			}
		case <-closeChan:
			conn.Close()
			relay.Close()
			return
		}
	}
}
func stream(c net.Conn, ct ConnectionType, mc chan Message, cc chan struct{}) {
	buf := bufio.NewReader(c)
	for {
		line, err := buf.ReadBytes('\n')
		if err != nil {
			cc <- struct{}{}
			return
		}

		newMessage := replaceToken(line)
		mc <- Message{Payload: newMessage, SenderType: ct}
	}
}
