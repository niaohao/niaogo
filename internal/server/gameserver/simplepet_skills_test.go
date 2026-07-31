package gameserver

import (
	"encoding/binary"
	"testing"

	"niaohao/server/internal/store"
)

func TestNoteReadyToFightPlayerSkills(t *testing.T) {
	st := &BattleState{
		PlayerPetID:     4,
		PlayerLevel:     5,
		PlayerName:      "伊优",
		PlayerCatchTime: 1768449796,
		PlayerHP:        21,
		PlayerMaxHP:     21,
		PlayerSkills: [][2]uint32{{10004, 35}, {10001, 40}},
		EnemyID:      4150,
		EnemyLevel:   20,
		EnemyName:    "拂晓兔",
		EnemyHP:      200,
		EnemyMaxHP:   200,
		EnemySkills:  [][2]uint32{{10001, 35}},
	}
	body := buildNoteReadyToFight(10002, "尼奥号", st, nil)
	if len(body) != 228 {
		t.Fatalf("len=%d want 228", len(body))
	}
	off := 32
	pet := body[off : off+80]
	skillNum := binary.BigEndian.Uint32(pet[16:20])
	s0 := binary.BigEndian.Uint32(pet[20:24])
	s1 := binary.BigEndian.Uint32(pet[28:32])
	ct := binary.BigEndian.Uint32(pet[52:56])
	t.Logf("player skillNum=%d skills=%d,%d catch=%d", skillNum, s0, s1, ct)
	if skillNum != 2 || s0 != 10004 || s1 != 10001 || ct != 1768449796 {
		t.Fatalf("bad player simple pet")
	}
}

func TestSkillsFromPetKeepsCategory4(t *testing.T) {
	p := &store.Pet{PetID: 4, Skills: []int{10004, 20002}}
	s := &Server{}
	out := s.skillsFromPet(p)
	if len(out) < 2 {
		t.Fatalf("want both skills, got %v", out)
	}
	hasProp := false
	for _, sk := range out {
		if sk[0] == 20002 {
			hasProp = true
		}
	}
	if !hasProp {
		t.Fatalf("category-4 20002 must be in fight skills: %v", out)
	}
}

func TestBuildPetInfoKeepsCategory4(t *testing.T) {
	p := &store.Pet{CatchTime: 1768449796, PetID: 4, Name: "伊优", Level: 5, DV: 20, Skills: []int{10004, 20002}}
	b := buildPetInfo(p)
	off := 4 + 16 + 24 + 28 + 24
	skillNum := binary.BigEndian.Uint32(b[off : off+4])
	s0 := binary.BigEndian.Uint32(b[off+4 : off+8])
	s1 := binary.BigEndian.Uint32(b[off+12 : off+16])
	// 背包/唤醒仪/进战均保留属性技
	if skillNum != 2 || s0 != 10004 || s1 != 20002 {
		t.Fatalf("PetInfo must keep 20002: skillNum=%d s0=%d s1=%d", skillNum, s0, s1)
	}
}

func TestFillPetSkillsKeepsCategory4(t *testing.T) {
	p := &store.Pet{PetID: 4, Level: 5, Skills: []int{10004, 20002}}
	s := &Server{}
	if defaultSkillCatalog != nil {
		s.cfg.Catalog = defaultSkillCatalog
	}
	_ = s.fillPetSkillsUpToFour(p)
	hasProp := false
	for _, sid := range p.Skills {
		if sid == 20002 {
			hasProp = true
		}
	}
	if !hasProp {
		t.Fatalf("fill must keep category-4 20002, got %v", p.Skills)
	}
}
