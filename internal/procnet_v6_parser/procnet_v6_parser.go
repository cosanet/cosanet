package procnet_v6_parser

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// parseSnmp6Line parses a single line from /proc/net/snmp6.
// It uses the first occurrence of the character '6' as separator between section and counter name.
func parseSnmp6Line(line string) (string, string, int, error) {
	idx := strings.Index(line, "6")
	if idx == -1 || idx == len(line)-1 {
		return "", "", 0, fmt.Errorf("no '6' found or nothing after '6'")
	}
	section := strings.TrimSpace(line[:idx+1])
	rest := strings.TrimSpace(line[idx+1:])
	fields := strings.Fields(rest)
	if len(fields) != 2 {
		return "", "", 0, fmt.Errorf("malformed snmp6 line: %s", line)
	}
	counterName := fields[0]
	val, err := strconv.Atoi(fields[1])
	if err != nil {
		return "", "", 0, err
	}
	return section, counterName, val, nil
}

// ParseSnmp6FromScanner parses /proc/net/snmp6 contents from a bufio.Scanner.
// It returns a nested map: section → field → int.
func parseV6FromScanner(scanner *bufio.Scanner) (map[string]map[string]int, error) {
	result := make(map[string]map[string]int)
	for scanner.Scan() {
		line := scanner.Text()
		section, counterName, val, err := parseSnmp6Line(line)
		if err != nil {
			continue // skip malformed lines
		}
		if _, ok := result[section]; !ok {
			result[section] = make(map[string]int)
		}
		result[section][counterName] = val
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// ParseSnmp6File opens the file and passes the scanner to the parser.
func ParseV6File(filename string) (map[string]map[string]int, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	return parseV6FromScanner(scanner)
}
