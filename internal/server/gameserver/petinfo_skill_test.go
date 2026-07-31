package gameserver

import (
	"encoding/binary"
	"testing"

	"niaohao/server/internal/store"
)

func TestBuildPetInfoSkillLayout(t *testing.T) {
	p := &store.Pet{CatchTime: 1768449796, PetID: 4, Name: "伊优", Level: 5, DV: 20, Skills: []int{10004, 20002}}
	b := buildPetInfo(p)
	if len(b) != 162 {
		t.Fatalf("len=%d want 162", len(b))
	}
	// skip to skillNum: id4+name16+dv..next 6*4=24 + hp/stats 7*4=28 + ev 6*4=24 = 4+16+24+28+24 = 96
	off := 4 + 16 + 24 + 28 + 24
	skillNum := binary.BigEndian.Uint32(b[off : off+4])
	off += 4
	t.Logf("skillNum=%d", skillNum)
	for i := 0; i < 4; i++ {
		id := binary.BigEndian.Uint32(b[off : off+4])
		pp := binary.BigEndian.Uint32(b[off+4 : off+8])
		off += 8
		t.Logf("skill[%d]=%d pp=%d", i, id, pp)
	}
	ct := binary.BigEndian.Uint32(b[off : off+4])
	t.Logf("catchTime=%d len=%d", ct, len(b))
	if skillNum != 2 {
		t.Fatalf("skillNum=%d want 2 (keep category-4 for bag/awakener)", skillNum)
	}
}
