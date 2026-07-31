package gameserver

import (
	"encoding/binary"
	"testing"
	"niaohao/server/internal/store"
)

func TestEmptySkillsCatchTimeIntact(t *testing.T) {
	petInfoForceEmptySkills = true
	defer func() { petInfoForceEmptySkills = false }()
	p := &store.Pet{CatchTime: 1768449796, PetID: 4, Name: "伊优", Level: 5, DV: 20, Skills: []int{10004, 20002}}
	b := buildPetInfo(p)
	if len(b) != 162 {
		t.Fatalf("len=%d", len(b))
	}
	skillNum := binary.BigEndian.Uint32(b[96:100])
	ct := binary.BigEndian.Uint32(b[132:136])
	t.Logf("skillNum=%d catch=%d", skillNum, ct)
	if skillNum != 0 {
		t.Fatalf("skillNum=%d want 0", skillNum)
	}
	if ct != 1768449796 {
		t.Fatalf("catch=%d", ct)
	}
}
