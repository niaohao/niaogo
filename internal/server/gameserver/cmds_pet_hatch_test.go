package gameserver

import (
	"encoding/binary"
	"testing"
)

func TestPushBossMonster8004PetLayout(t *testing.T) {
	b := buildBossMonster8004Body(0, 41, 12345, 0, 0)
	if len(b) != 16 {
		t.Fatalf("len=%d", len(b))
	}
	if binary.BigEndian.Uint32(b[4:8]) != 41 || binary.BigEndian.Uint32(b[8:12]) != 12345 {
		t.Fatalf("%v", b)
	}
	if binary.BigEndian.Uint32(b[12:16]) != 0 {
		t.Fatal("itemCount")
	}
}
