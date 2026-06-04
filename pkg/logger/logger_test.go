package logger

import (
	"bytes"
	"strings"
	"testing"
)

func TestJournalPriorityPrefix(t *testing.T) {
	tests := []struct {
		name  string
		log   func(string, ...any)
		want  string
		level string
	}{
		{name: "debug", log: Debug, want: "<7>", level: "debug"},
		{name: "info", log: Info, want: "<6>", level: "debug"},
		{name: "warn", log: Warn, want: "<4>", level: "debug"},
		{name: "error", log: Error, want: "<3>", level: "debug"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			Init(Config{Level: tt.level, Format: "text", Output: &out})

			tt.log(tt.name)

			if got := out.String(); !strings.HasPrefix(got, tt.want) {
				t.Fatalf("log prefix = %q, want prefix %q", got, tt.want)
			}
		})
	}
}

func TestJournalPriorityWriterPrefixesEachLine(t *testing.T) {
	var out bytes.Buffer
	w := &journalPriorityWriter{output: &out, prefix: "<6>"}

	n, err := w.Write([]byte("one\ntwo\n"))
	if err != nil {
		t.Fatal(err)
	}
	if n != len("one\ntwo\n") {
		t.Fatalf("written bytes = %d, want %d", n, len("one\ntwo\n"))
	}

	if got, want := out.String(), "<6>one\n<6>two\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestJournaldRedundantTimeIsOmitted(t *testing.T) {
	var out bytes.Buffer
	Init(Config{Level: "info", Format: "text", Output: &out})

	Info("hello")

	got := out.String()
	if strings.Contains(got, "time=") {
		t.Fatalf("log output contains time: %q", got)
	}
	if !strings.Contains(got, "level=INFO") {
		t.Fatalf("log output = %q, want level", got)
	}
	if !strings.Contains(got, "msg=hello") {
		t.Fatalf("log output = %q, want message", got)
	}
}
