package sdk

import (
	"testing"

	"github.com/nolouch/gcode/internal/bus"
)

func TestDecodeWireEventTypedPayload(t *testing.T) {
	payload := `{"type":"text.delta","session_id":"s1","message_id":"m1","payload":{"delta":"hi"}}`
	e, err := decodeWireEvent(payload)
	if err != nil {
		t.Fatalf("decodeWireEvent: %v", err)
	}
	if e.Type != bus.EventTextDelta {
		t.Fatalf("unexpected type: %s", e.Type)
	}
	p, ok := e.Payload.(bus.TextDeltaPayload)
	if !ok {
		t.Fatalf("payload type = %T, want bus.TextDeltaPayload", e.Payload)
	}
	if p.Delta != "hi" {
		t.Fatalf("delta = %q, want hi", p.Delta)
	}
}
