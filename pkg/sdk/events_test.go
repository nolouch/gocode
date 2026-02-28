package sdk

import (
	"testing"

	"github.com/nolouch/gcode/internal/bus"
)

func TestDecodeWireEventTypedPayload(t *testing.T) {
	payload := `{"type":"message.part.done","session_id":"s1","message_id":"m1","payload":{"part_id":"p1","part_type":"reasoning","duration_ms":123}}`
	e, err := decodeWireEvent(payload)
	if err != nil {
		t.Fatalf("decodeWireEvent: %v", err)
	}
	if e.Type != bus.EventPartDone {
		t.Fatalf("unexpected type: %s", e.Type)
	}
	p, ok := e.Payload.(bus.PartDonePayload)
	if !ok {
		t.Fatalf("payload type = %T, want bus.PartDonePayload", e.Payload)
	}
	if p.PartID != "p1" || string(p.PartType) != "reasoning" || p.DurationMs != 123 {
		t.Fatalf("unexpected payload: %#v", p)
	}
}

func TestDecodeWireEventPartDeltaPayload(t *testing.T) {
	payload := `{"type":"message.part.delta","session_id":"s1","message_id":"m1","payload":{"part_id":"p1","part_type":"reasoning","field":"text","delta":"thinking"}}`
	e, err := decodeWireEvent(payload)
	if err != nil {
		t.Fatalf("decodeWireEvent: %v", err)
	}
	if e.Type != bus.EventPartDelta {
		t.Fatalf("unexpected type: %s", e.Type)
	}
	p, ok := e.Payload.(bus.PartDeltaPayload)
	if !ok {
		t.Fatalf("payload type = %T, want bus.PartDeltaPayload", e.Payload)
	}
	if p.PartID != "p1" || string(p.PartType) != "reasoning" || p.Delta != "thinking" {
		t.Fatalf("unexpected payload: %#v", p)
	}
}
