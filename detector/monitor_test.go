package main

import "testing"

func TestParseAccessLogLine(t *testing.T) {
	line := []byte(`{"source_ip":"198.51.100.23","timestamp":"2026-04-25T12:00:00+00:00","method":"GET","path":"/index.php/login","status":404,"response_size":1234}`)
	entry, err := ParseAccessLogLine(line)
	if err != nil {
		t.Fatalf("ParseAccessLogLine returned error: %v", err)
	}
	if entry.SourceIP != "198.51.100.23" {
		t.Fatalf("SourceIP = %q", entry.SourceIP)
	}
	if entry.Status != 404 {
		t.Fatalf("Status = %d", entry.Status)
	}
	if entry.ResponseSize != 1234 {
		t.Fatalf("ResponseSize = %d", entry.ResponseSize)
	}
	if entry.ParsedTime.IsZero() {
		t.Fatal("ParsedTime was not populated")
	}
}

func TestParseAccessLogLineRejectsMissingFields(t *testing.T) {
	_, err := ParseAccessLogLine([]byte(`{"source_ip":"198.51.100.23"}`))
	if err == nil {
		t.Fatal("expected parse error for missing fields")
	}
}
