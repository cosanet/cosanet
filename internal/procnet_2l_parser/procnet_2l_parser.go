package procnet_2l_parser

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ParseSection parses a pair of lines: a header line and a value line.
// It returns the section name and a map of field -> int value.
func parseSectionCouple(headerLine, valueLine string) (string, map[string]int, error) {
	headerFields := strings.Fields(headerLine)
	valueFields := strings.Fields(valueLine)

	if len(headerFields) == 0 || len(valueFields) == 0 || headerFields[0] != valueFields[0] {
		return "", nil, fmt.Errorf("malformed section lines")
	}

	section := strings.TrimSuffix(headerFields[0], ":")
	counters := make(map[string]int)
	for i := 1; i < len(headerFields) && i < len(valueFields); i++ {
		val, err := strconv.Atoi(valueFields[i])
		if err != nil {
			// skip invalid values but continue parsing others
			continue
		}
		counters[headerFields[i]] = val
	}
	return section, counters, nil
}

// ParseNetstatFromScanner parses /proc/net/netstat contents from a bufio.Scanner.
// It returns a nested map: section → field → int.
func parse2LFromScanner(scanner *bufio.Scanner) (map[string]map[string]int, error) {
	result := make(map[string]map[string]int)

	for scanner.Scan() {
		headerLine := scanner.Text()
		if !scanner.Scan() {
			break // no matching value line for header
		}
		valueLine := scanner.Text()

		section, counters, err := parseSectionCouple(headerLine, valueLine)
		if err != nil {
			// skip malformed section but keep parsing
			continue
		}
		result[section] = counters
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// Parse2LFile opens the file and passes the scanner to the parser.
func Parse2LFile(filename string) (map[string]map[string]int, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	return parse2LFromScanner(scanner)
}
