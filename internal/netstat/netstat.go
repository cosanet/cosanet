package netstat

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Very very very very VERY inspired for the marvelous work of cakturk

const (
	pathTCPTab      = "/proc/net/tcp"
	pathTCP6Tab     = "/proc/net/tcp6"
	pathUDPTab      = "/proc/net/udp"
	pathUDP6Tab     = "/proc/net/udp6"
	pathICMPTab     = "/proc/net/icmp"
	pathICMP6Tab    = "/proc/net/icmp6"
	pathUDPLiteTab  = "/proc/net/udplite"
	pathUDPLite6Tab = "/proc/net/udplite6"
	pathRAWTab      = "/proc/net/raw"
	pathRAW6Tab     = "/proc/net/raw6"
)

// Very very very very VERY inspired for the marvelous work of cakturk
// Socket states
const (
	Established SkState = 0x01
	SynSent             = 0x02
	SynRecv             = 0x03
	FinWait1            = 0x04
	FinWait2            = 0x05
	TimeWait            = 0x06
	Close               = 0x07
	CloseWait           = 0x08
	LastAck             = 0x09
	Listen              = 0x0a
	Closing             = 0x0b
)

// Very very very very VERY inspired for the marvelous work of cakturk
var skStates = [...]string{
	"UNKNOWN",
	"ESTABLISHED",
	"SYN_SENT",
	"SYN_RECV",
	"FIN_WAIT1",
	"FIN_WAIT2",
	"TIME_WAIT",
	"CLOSE", // CLOSE
	"CLOSE_WAIT",
	"LAST_ACK",
	"LISTEN",
	"CLOSING",
}

// Very very very very VERY inspired for the marvelous work of cakturk
// Errors returned by gonetstat
var (
	ErrNotEnoughFields = errors.New("gonetstat: not enough fields in the line")
)

// SkState type represents socket connection state
type SkState uint8

func (s SkState) String() string {
	return skStates[s]
}

type SocketStats map[string]int

// Very very very very VERY inspired for the marvelous work of cakturk
func parseSocktab(r io.Reader) (SocketStats, error) {
	br := bufio.NewScanner(r)
	stats := make(SocketStats)

	// Discard title
	br.Scan()

	for br.Scan() {
		line := br.Text()
		// Skip comments
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		if len(fields) < 12 {
			return nil, fmt.Errorf("netstat: not enough fields: %v, %v", len(fields), fields)
		}

		u, err := strconv.ParseUint(fields[3], 16, 8)
		if err != nil {
			return nil, err
		}

		state := SkState(u).String()
		stats[state]++
	}
	return stats, br.Err()
}

func parseSockTabFile(filename string) (SocketStats, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return parseSocktab(file)
}

// TCPSocks returns a slice of active TCP sockets containing only those
// elements that satisfy the accept function
func TCPStats() (SocketStats, error) {
	return parseSockTabFile(pathTCPTab)
}

// TCP6Socks returns a slice of active TCP IPv4 sockets containing only those
// elements that satisfy the accept function
func TCP6Stats() (SocketStats, error) {
	return parseSockTabFile(pathTCP6Tab)
}

// UDPSocks returns a slice of active UDP sockets containing only those
// elements that satisfy the accept function
func UDPStats() (SocketStats, error) {
	return parseSockTabFile(pathUDPTab)
}

// UDP6Socks returns a slice of active UDP IPv6 sockets containing only those
// elements that satisfy the accept function
func UDP6Stats() (SocketStats, error) {
	return parseSockTabFile(pathUDP6Tab)
}

// ICMPSocks returns a slice of active ICMP sockets containing only those
// elements that satisfy the accept function
func ICMPStats() (SocketStats, error) {
	return parseSockTabFile(pathICMPTab)
}

// ICMP6Socks returns a slice of active ICMP IPv6 sockets containing only those
// elements that satisfy the accept function
func ICMP6Stats() (SocketStats, error) {
	return parseSockTabFile(pathICMP6Tab)
}

// UDPLiteSocks returns a slice of active UDPLite sockets containing only those
// elements that satisfy the accept function
func UDPLiteStats() (SocketStats, error) {
	return parseSockTabFile(pathUDPLiteTab)
}

// UDPLite6Socks returns a slice of active UDPLite IPv6 sockets containing only those
// elements that satisfy the accept function
func UDPLite6Stats() (SocketStats, error) {
	return parseSockTabFile(pathUDPLite6Tab)
}

// RAWSocks returns a slice of active RAW sockets containing only those
// elements that satisfy the accept function
func RAWStats() (SocketStats, error) {
	return parseSockTabFile(pathRAWTab)
}

// RAW6Socks returns a slice of active RAW IPv6 sockets containing only those
// elements that satisfy the accept function
func RAW6Stats() (SocketStats, error) {
	return parseSockTabFile(pathRAW6Tab)
}
