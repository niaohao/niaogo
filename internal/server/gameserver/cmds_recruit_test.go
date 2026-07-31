package gameserver

import (
	"encoding/binary"
	"testing"
)

func TestRecruitStatesFromMask(t *testing.T) {
	out := recruitStatesFromMask(0b0101) // slot1+slot3 claimed
	if len(out) != 16 {
		t.Fatalf("len=%d", len(out))
	}
	if binary.BigEndian.Uint32(out[0:4]) != 1 {
		t.Fatalf("slot1 want 1")
	}
	if binary.BigEndian.Uint32(out[4:8]) != 0 {
		t.Fatalf("slot2 want 0")
	}
	if binary.BigEndian.Uint32(out[8:12]) != 1 {
		t.Fatalf("slot3 want 1")
	}
	if binary.BigEndian.Uint32(out[12:16]) != 0 {
		t.Fatalf("slot4 want 0")
	}
}
