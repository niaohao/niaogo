package gameserver

import (
	"encoding/binary"
	"testing"

	"niaohao/server/internal/store"
)

func TestBuildPetInfoEmptySkillsFallback(t *testing.T) {
	p := &store.Pet{CatchTime: 1768449796, PetID: 4, Name: "伊优", Level: 5, DV: 20, Skills: nil}
	b := buildPetInfo(p)
	off := 4 + 16 + 24 + 28 + 24
	skillNum := binary.BigEndian.Uint32(b[off : off+4])
	off += 4
	s0 := binary.BigEndian.Uint32(b[off : off+4])
	s1 := binary.BigEndian.Uint32(b[off+8 : off+12])
	t.Logf("skillNum=%d s0=%d s1=%d", skillNum, s0, s1)
	// 空技能回落 starterPets（含属性技 20002）；背包需完整展示
	if skillNum != 2 || s0 != 10004 || s1 != 20002 {
		t.Fatalf("fallback want starter skills: skillNum=%d s0=%d s1=%d", skillNum, s0, s1)
	}
}
