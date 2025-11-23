package main

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"unicode"
)

/*
- each message is a single line of ASCII text terminated by a newline char.
- clients can send multiple messages per connection
- servers may optionally strip trailing whitespace, such as carriage returnn chars
- all messages are raw ASCII text not wrapped in json or any format.

- on join
	- server asks client for their name
		- must be at least 1 char and always alphanumeric
		- must allow at least 16 chars for uname
		- can allow or reject duplicate names
		- if user requests an illegal name, the server may send an informative
		  error message to the client and the server must disconnect the client
		  without sending anything about the illegal user to any other client

- presence notification
	- once a user has a name, they are officially joined.
	- once joined, the server must broadcast their presense to other users
	- in addition, the server must send the new user a message that lists all
	  present users names, not including the new user, and not including any
	  user who has already left. Exact text is implementation defined, but must
	  start with a '*' and must contain the users names.
	- the server must send the message even if the room was empty

- chat messages
	- when a client sends a chat message to the server, the server must relay
	  to all other clients as the concatenation of:
	  	- [<sender_uname>] <message>
	- messages must allow for at least 1000 chars

- user leaves
	- when a joined user is disconnected from the server for any reason, the
	  server must send all other users a message to inform them that the user
	  has left. The exact text is implementation defined, but must start with '*'
	  and must contain the users name.
	- if a client that has not yet joined disconnects from the server, the
	  server must not send any messages about that client to other clients

*/

var (
	PORT           = 8899
	INITIAL_PROMPT = "Welcome to budgetchat! What shall I call you?\n"
)

type User struct {
	Name string
	Conn *net.Conn
}

func validateName(name string) error {
	nl := len(name)
	// must have at least 1, but no more than 20
	if nl == 0 || nl > 20 {
		return fmt.Errorf("Invalid name length")
	}

	for _, ch := range name {
		r := rune(ch)
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == 10 {
			continue
		}
		fmt.Println(r)
		return fmt.Errorf("Name must be unicode")
	}
	return nil
}

type Connections struct {
	ConMap         map[*net.Conn]User
	NumConnections uint32
	mutex          sync.Mutex
}

func (c *Connections) Add(u User) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.ConMap[u.Conn] = u
	c.NumConnections++
}
func (c *Connections) Remove(conn *net.Conn) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	delete(c.ConMap, conn)
	c.NumConnections--
}

func (c *Connections) Get(conn *net.Conn) (User, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	user, ok := c.ConMap[conn]
	if !ok {
		return user, fmt.Errorf("This user never joined.")
	}
	return user, nil
}

type Room struct {
	JoinedUsers []User
	Joined      uint32
	mutex       sync.Mutex
}

func (r *Room) Add(u User) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	var names []string
	// ensure username uniqueness
	for _, user := range r.JoinedUsers {
		if user.Name == u.Name {
			return fmt.Errorf("Username already exists: %s", u.Name)
		}
		names = append(names, user.Name)
	}
	r.JoinedUsers = append(r.JoinedUsers, u)
	r.Joined++

	combined_names := strings.Join(names, ",")

	for _, user := range r.JoinedUsers {
		// send welcome message
		if user.Name == u.Name {
			(*user.Conn).Write([]byte(fmt.Sprintf("* Room contains: %s\n", combined_names)))
		} else { // send user joined message
			(*user.Conn).Write([]byte(fmt.Sprintf("* %s has entered the room\n", u.Name)))
		}
	}
	return nil
}

/*
Remove a user from a room.
Sends a notif to all joined user that the user has left.
*/
func (r *Room) Remove(u User) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	removeIdx := -1
	for i, user := range r.JoinedUsers {
		if user.Name == u.Name {
			removeIdx = i
			continue
		}
		(*user.Conn).Write([]byte(fmt.Sprintf("* %s has left the room\n", u.Name)))
	}
	r.JoinedUsers = append(r.JoinedUsers[:removeIdx], r.JoinedUsers[removeIdx+1:]...)
	r.Joined--
}

func (r *Room) SendMessage(message []byte, sender User) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	for _, user := range r.JoinedUsers {
		// Skip sending to the sender
		if user.Name == sender.Name {
			continue
		}
		(*user.Conn).Write(message)
	}
}

func main() {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", PORT))
	if err != nil {
		fmt.Println("Failed to start server: ", err)
		os.Exit(1)
	}
	var room Room
	var connections Connections
	connections.ConMap = make(map[*net.Conn]User)
	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("Failed to accept connection from : ", conn.RemoteAddr())
			continue
		}

		go handle(conn, &room, &connections)
	}
}

func handle(conn net.Conn, room *Room, connections *Connections) {
	defer func() {
		fmt.Println("Closing connection for: ", conn.RemoteAddr())
		conn.Close()
		u, err := connections.Get(&conn)
		fmt.Println(u)
		if err != nil {
			fmt.Println(err)
			return
		}
		room.Remove(u)
	}()

	// prompt user for name
	_, err := conn.Write([]byte(INITIAL_PROMPT))
	if err != nil {
		fmt.Println("An error occurred after initial prompt: ", err)
		return
	}

	reader := bufio.NewReader(conn)
	// the users first message should be their name
	n, _ := reader.ReadBytes('\n')
	n = bytes.TrimSpace(n)
	name := string(n)
	if err = validateName(name); err != nil {
		fmt.Println(err, " ", name)
		return
	}
	user := User{Name: name, Conn: &conn}
	// add the user to the room
	err = room.Add(user)
	if err != nil {
		fmt.Println("Failed to add user to room: ", err)
		return
	}
	connections.Add(user)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			fmt.Println("Failed to read from user: ", user.Name)
			return
		}
		intro := []byte(fmt.Sprintf("[%s] ", user.Name))
		message := append(intro, line...)
		room.SendMessage(message, user)
	}

}
