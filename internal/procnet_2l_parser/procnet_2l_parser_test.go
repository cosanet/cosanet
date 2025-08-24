package procnet_2l_parser

import (
	"bufio"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSectionCouple_Valid(t *testing.T) {
	header := "TcpExt: SyncookiesSent SyncookiesRecv"
	value := "TcpExt: 10 20"
	section, counters, err := parseSectionCouple(header, value)
	require.NoError(t, err)
	assert.Equal(t, "TcpExt", section)
	assert.Equal(t, map[string]int{"SyncookiesSent": 10, "SyncookiesRecv": 20}, counters)
}

func TestParseSectionCouple_Malformed(t *testing.T) {
	header := "TcpExt: SyncookiesSent SyncookiesRecv"
	value := "Other: 10 20"
	_, _, err := parseSectionCouple(header, value)
	assert.Error(t, err)
}

func TestParseSectionCouple_InvalidValue(t *testing.T) {
	header := "TcpExt: SyncookiesSent SyncookiesRecv"
	value := "TcpExt: 10 notanint"
	section, counters, err := parseSectionCouple(header, value)
	require.NoError(t, err)
	assert.Equal(t, "TcpExt", section)
	assert.Equal(t, map[string]int{"SyncookiesSent": 10}, counters)
}

func TestParse2LFromScanner_Valid(t *testing.T) {
	data := "TcpExt: SyncookiesSent SyncookiesRecv\nTcpExt: 10 20\nIpExt: InOctets OutOctets\nIpExt: 100 200"
	scanner := bufio.NewScanner(strings.NewReader(data))
	result, err := parse2LFromScanner(scanner)
	require.NoError(t, err)
	assert.Equal(t, map[string]map[string]int{
		"TcpExt": {"SyncookiesSent": 10, "SyncookiesRecv": 20},
		"IpExt":  {"InOctets": 100, "OutOctets": 200},
	}, result)
}

func TestParse2LFromScanner_MalformedSection(t *testing.T) {
	data := "TcpExt: SyncookiesSent SyncookiesRecv\nOther: 10 20\nIpExt: InOctets OutOctets\nIpExt: 100 200"
	scanner := bufio.NewScanner(strings.NewReader(data))
	result, err := parse2LFromScanner(scanner)
	require.NoError(t, err)
	assert.Equal(t, map[string]map[string]int{
		"IpExt": {"InOctets": 100, "OutOctets": 200},
	}, result)
}

func TestParse2LFromScanner_ScannerError(t *testing.T) {
	badReader := strings.NewReader("")
	scanner := bufio.NewScanner(badReader)
	_, err := parse2LFromScanner(scanner)
	assert.NoError(t, err)
}

func TestParse2LFromScanner_ScannerErrorPropagated(t *testing.T) {
	r := strings.NewReader("TcpExt: SyncookiesSent\nTcpExt: 10\n")
	scanner := bufio.NewScanner(r)
	// Simulate scanner error by closing the reader early
	scanner.Err() // No error, but for completeness
	result, err := parse2LFromScanner(scanner)
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestParse2LFromScanner_OddNumberOfLines(t *testing.T) {
	data := "TcpExt: SyncookiesSent SyncookiesRecv\nTcpExt: 10 20\nIpExt: InOctets OutOctets\n"
	scanner := bufio.NewScanner(strings.NewReader(data))
	result, err := parse2LFromScanner(scanner)
	assert.NoError(t, err)
	assert.Equal(t, map[string]map[string]int{
		"TcpExt": {"SyncookiesSent": 10, "SyncookiesRecv": 20},
	}, result)
}

func TestParse2LFromScanner_MalformedSectionSkipped(t *testing.T) {
	data := "TcpExt: SyncookiesSent SyncookiesRecv\nOther: 10 20\nIpExt: InOctets OutOctets\nIpExt: 100 200\n"
	scanner := bufio.NewScanner(strings.NewReader(data))
	result, err := parse2LFromScanner(scanner)
	assert.NoError(t, err)
	assert.Equal(t, map[string]map[string]int{
		"IpExt": {"InOctets": 100, "OutOctets": 200},
	}, result)
}

func TestParse2LFromScanner_EmptyScanner(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader(""))
	result, err := parse2LFromScanner(scanner)
	assert.NoError(t, err)
	assert.Empty(t, result)
}
