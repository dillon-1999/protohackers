package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
)

var (
	PORT               = 8899
	ErrMalformedNumber = errors.New("number must be numerical")
	ErrNotInteger      = errors.New("number must be an integer")
)

type JsonPayload struct {
	Method *string       `json:"method"`
	Number *StrictBigInt `json:"number"`
}

type JsonResponse struct {
	Method string `json:"method"`
	Prime  bool   `json:"prime"`
}

type StrictBigInt struct {
	Num *big.Int
}

// must be > 0
func (s *StrictBigInt) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		fmt.Println("data is of length 0")
		return ErrMalformedNumber
	}

	if data[0] == '"' {
		fmt.Println("data starts with \"")
		return ErrMalformedNumber
	}

	if bytes.ContainsAny(data, ".") {
		return ErrNotInteger
	}

	bi := new(big.Int)
	_, ok := bi.SetString(string(data), 10)
	if !ok {
		fmt.Println("not OK! ", string(data))
		return ErrMalformedNumber
	}
	*s = StrictBigInt{Num: bi}
	return nil
}

func main() {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", PORT))
	if err != nil {
		fmt.Println("Failed to listen to port: ", PORT)
		os.Exit(1)
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("Error accepting incoming connection: ", err.Error())
		}
		fmt.Printf("Accepting connection from: %s\n", conn.RemoteAddr())
		go handle(conn)
	}
}

// malformed:
// - method not isPrime
// - err unmarshaling that isnt:
//   - ErrNotPositiveInteger
//
// bad, but not malformed:
func validatePayload(b []byte) (JsonPayload, error) {
	var jp JsonPayload
	err := json.Unmarshal(b, &jp)

	if jp.Method == nil || *jp.Method != "isPrime" {
		fmt.Println(jp.Method)
		return jp, fmt.Errorf("Invalid Method")
	}

	if jp.Number == nil && !errors.Is(err, ErrNotInteger) {
		return jp, fmt.Errorf("Invalid Number")
	}
	return jp, err
}

/*
problems:
- cant read in as string, because i wont know the difference between "1" and 1
  - "1" is invalid, but 1 would not be

- cant read in as json.Number, same as above ^
- cant read in as bit.Int, because it would drop the precision
- can
*/

func handle(conn net.Conn) {
	defer func() {
		fmt.Println("Closing connection: ")
		conn.Close()
	}()
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}
		fmt.Println("Line: ", string(line))
		jp, err := validatePayload(line)
		// if the error we saw wasn't malformed, don't send a malformed response.
		// values caught in this: negative & floats
		if err != nil && !errors.Is(err, ErrNotInteger) {
			fmt.Println("Failed in validate Payload", err)
			conn.Write([]byte("You gave me bad input..."))
			return
		}

		prime := false
		if !errors.Is(err, ErrNotInteger) && jp.Number != nil {
			prime = jp.Number.Num.ProbablyPrime(20)
		}
		jr := JsonResponse{Method: "isPrime", Prime: prime}
		b, err := json.Marshal(jr)
		if err != nil {
			fmt.Println("failed to convert response prior to sending")
			return
		}
		b = append(b, '\n')
		conn.Write(b)
	}
}
