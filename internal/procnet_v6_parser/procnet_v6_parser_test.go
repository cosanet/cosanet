package procnet_v6_parser

import (
	"bufio"
	"strings"
	"testing"
)

func TestParseSnmp6Line(t *testing.T) {
	tests := []struct {
		line        string
		wantSection string
		wantCounter string
		wantValue   int
		wantErr     bool
	}{
		{"Icmp6InMsgs     42", "Icmp6", "InMsgs", 42, false},
		{"Tcp6ActiveOpens 123", "Tcp6", "ActiveOpens", 123, false},
		{"Udp6InDatagrams 999", "Udp6", "InDatagrams", 999, false},
		{"No6Counter      1", "No6", "Counter", 1, false},
		{"NoSixCounter      1", "", "", 0, true},
		{"MalformedLine", "", "", 0, true},
		{"MalformedLine6Thing", "", "", 0, true},
		{"Section6Counter notanint", "", "", 0, true},
	}
	for _, tt := range tests {
		section, counter, val, err := parseSnmp6Line(tt.line)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseSnmp6Line(%q) error = %v, wantErr %v", tt.line, err, tt.wantErr)
		}
		if section != tt.wantSection {
			t.Errorf("parseSnmp6Line(%q) section = %q, want %q", tt.line, section, tt.wantSection)
		}
		if counter != tt.wantCounter {
			t.Errorf("parseSnmp6Line(%q) counter = %q, want %q", tt.line, counter, tt.wantCounter)
		}
		if val != tt.wantValue {
			t.Errorf("parseSnmp6Line(%q) value = %d, want %d", tt.line, val, tt.wantValue)
		}
	}
}

func TestParseSnmp6FromScanner(t *testing.T) {
	input := `Icmp6InMsgs 42\nTcp6ActiveOpens 123\nUdp6InDatagrams 999\nMalformedLine\nSection6Counter notanint`
	scanner := bufio.NewScanner(strings.NewReader(strings.ReplaceAll(input, "\\n", "\n")))
	result, err := parseV6FromScanner(scanner)
	if err != nil {
		t.Fatalf("ParseSnmp6FromScanner error: %v", err)
	}
	if result["Icmp6"]["InMsgs"] != 42 {
		t.Errorf("Icmp6/InMsgs = %d, want 42", result["Icmp6"]["InMsgs"])
	}
	if result["Tcp6"]["ActiveOpens"] != 123 {
		t.Errorf("Tcp6/ActiveOpens = %d, want 123", result["Tcp6"]["ActiveOpens"])
	}
	if result["Udp6"]["InDatagrams"] != 999 {
		t.Errorf("Udp6/InDatagrams = %d, want 999", result["Udp6"]["InDatagrams"])
	}
	if _, ok := result["MalformedLine"]; ok {
		t.Errorf("MalformedLine should not be parsed")
	}
	if _, ok := result["Section6"]; ok {
		if _, ok2 := result["Section6"]["Counter"]; ok2 {
			t.Errorf("Section6/Counter should not be parsed due to value error")
		}
	}
}
