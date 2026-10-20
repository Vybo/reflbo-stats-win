package main

import (
	"bytes"
	"errors"
	"log"
	"testing"
)

type failing struct{}

func (failing) Write([]byte) (int, error) { return 0, errors.New("the handle is invalid") }

func TestTheLogReachesTheFileWhenStderrFails(t *testing.T) {
	var file bytes.Buffer
	l := log.New(logWriter(&file, failing{}), "", 0)
	l.Print("the LAN port is taken")
	if file.String() != "the LAN port is taken\n" {
		t.Fatalf("log.txt got %q", file.String())
	}
}
