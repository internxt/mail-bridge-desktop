package control

import (
	"bytes"
	"context"
	"encoding/binary"
	"strings"
	"testing"
)

func TestReadFrameRejectsOversizedFrame(t *testing.T) {
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], MaxFrameSize+1)
	_, err := readFrame(context.Background(), bytes.NewReader(header[:]))
	if err == nil || !strings.Contains(err.Error(), "invalid control frame size") {
		t.Fatalf("expected oversized frame error, got %v", err)
	}
}

func TestReadMessageRejectsUnknownFields(t *testing.T) {
	var framed bytes.Buffer
	if err := writeFrame(context.Background(), &framed, []byte(`{"type":"start_session","unknown":true}`)); err != nil {
		t.Fatal(err)
	}
	_, err := ReadMessage(context.Background(), &framed)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}
