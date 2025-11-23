package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
)

/*

Message Format:
Byte:  |  0  |  1     2     3     4  |  5     6     7     8  |
Type:  |char |         int32         |         int32         |

message[0] = I or Q (inserts / queries)
- if [0] is neigher 'I' or 'Q', behavior is undefined

The int32s,
message[1:5]
message[5:9]
are two twos compliment signed integers in big endian format.
What they represent differs depending on the type signifier (I/Q)
********************************************************************************
I:
Byte:  |  0  |  1     2     3     4  |  5     6     7     8  |
Type:  |char |         int32         |         int32         |
Value: | 'I' |       timestamp       |         price         |
[1, 5] -> timestamp in seconds since 00:00, 1st Jan 1970
[5, 9] -> price in pennies, of this clients asset at the given timestamp
Notes:cur out-of-order
- prices can be neg
- insertions may ocative, but its rare
- behavior is undefined if multiple prices with the same timestamp exist from the same client
********************************************************************************
Q:
Byte:  |  0  |  1     2     3     4  |  5     6     7     8  |
Type:  |char |         int32         |         int32         |
Value: | 'Q' |        mintime        |        maxtime        |
[1, 5] -> the earliest timestamp from the period
[5, 9] -> the latest timestamp of the period
From this query, the server must compute the mean of the inserted prices with
timestamps T, mintime <= T <= maxtime. If the mean is not an integer, it is acceptable
to either roudn up or down at the servers discretion.

The server must send the mean to the client as a single int32
Notes:
- if there are no samples in the timeframe, send 0
- if the mintime occures after maxtime, send 0

Overall Notes:
- when a client triggers undefined behavior, the server can do anything
	it likes for that client, but must not adversly affect other clients that
	did not trigger undefined behavior
*/

var (
	PORT = 8899
)

type Record struct {
	Price     int32
	Timestamp int32
}

type RecordSet struct {
	Sorted  bool
	Records []Record
}

const QUERY = 'Q'
const INSERT = 'I'

func main() {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", PORT))
	if err != nil {
		fmt.Println("Could not listen to address")
		os.Exit(1)
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("Failed to accept connection from: ", conn.RemoteAddr())
			continue
		}
		go handle(conn)
	}

}

func convertI32Slice(buf []byte) int32 {
	if len(buf) != 4 {
		panic("convert: byte slice must be exactly 4 bytes")
	}
	var i32 int32
	binary.Decode(buf, binary.BigEndian, &i32)
	return i32
}

func insert(timestamp int32, price int32, records *[]Record) {
	*records = append(*records, Record{Timestamp: timestamp, Price: price})
}

func query(mintime int32, maxtime int32, records []Record) int32 {
	var count int64
	var total int64
	for _, record := range records {
		// timestamp between min and max
		if record.Timestamp >= mintime && record.Timestamp <= maxtime {
			count++
			total += int64(record.Price)
		}
	}
	if count == 0 {
		return 0
	}
	return int32(total / count)
}

func handle(conn net.Conn) {
	defer func() {
		fmt.Println("Closing connection: ", conn.RemoteAddr())
		conn.Close()
	}()
	var rs RecordSet
	buf := make([]byte, 9)
	for {
		// buffer big enough to grab the whole query
		size, err := io.ReadFull(conn, buf)
		if err != nil {
			if err != io.EOF {
				fmt.Println("Failed to ReadFull: ", err)
			}
			return
		}
		if size != 9 {
			fmt.Println("Invalid message size: ", size)
			return
		}

		mt := buf[0]
		b1 := convertI32Slice(buf[1:5])
		b2 := convertI32Slice(buf[5:9])

		switch mt {
		case INSERT:
			insert(b1, b2, &rs.Records)
			if rs.Sorted {
				rs.Sorted = false
			}
		case QUERY:
			// before you query, ensure it's sorted
			if !rs.Sorted {
				sort.Slice(rs.Records, func(i, j int) bool {
					return (rs.Records)[i].Timestamp < (rs.Records)[j].Timestamp
				})
				rs.Sorted = true
			}
			mean := query(b1, b2, rs.Records)
			err = binary.Write(conn, binary.BigEndian, mean)
			if err != nil {
				fmt.Println("Error writing binary: ", err)
				return
			}
		default:
			fmt.Println("fell into default.. somethings wrong")
			return
		}

	}
}
