package gameserver

import (
	"encoding/binary"
	"testing"

	"niaohao/server/internal/store"
	"niaohao/server/internal/tableloader"
)

func TestBuildNoteUpdateSkillLayout(t *testing.T) {
	b := buildNoteUpdateSkill(12345, []int{10002}, []int{10003, 10011})
	if len(b) != 4+4+4+4+4+8 {
		t.Fatalf("len=%d", len(b))
	}
	if binary.BigEndian.Uint32(b[0:4]) != 1 {
		t.Fatal("count")
	}
	if binary.BigEndian.Uint32(b[4:8]) != 12345 {
		t.Fatal("catch")
	}
	if binary.BigEndian.Uint32(b[8:12]) != 1 || binary.BigEndian.Uint32(b[12:16]) != 2 {
		t.Fatalf("counts active/unactive")
	}
	if binary.BigEndian.Uint32(b[16:20]) != 10002 {
		t.Fatal("active0")
	}
	if binary.BigEndian.Uint32(b[20:24]) != 10003 {
		t.Fatal("unactive0")
	}
}

func TestGetPetSkillEmptyBodyNotNil(t *testing.T) {
	out := make([]byte, 4)
	if len(out) < 4 || binary.BigEndian.Uint32(out) != 0 {
		t.Fatal(out)
	}
}

func TestPetSkillSwitchParse20(t *testing.T) {
	// 模拟客户端 20B：catch + count=1 + slot=0 + oldSkill + newSkill
	body := make([]byte, 20)
	binary.BigEndian.PutUint32(body[0:4], 1784868204)
	binary.BigEndian.PutUint32(body[4:8], 1)
	binary.BigEndian.PutUint32(body[8:12], 0)
	binary.BigEndian.PutUint32(body[12:16], 10001)
	binary.BigEndian.PutUint32(body[16:20], 10002)
	count := int(binary.BigEndian.Uint32(body[4:8]))
	if count != 1 || len(body) < 8+count*8 {
		t.Fatal("layout")
	}
	slot := int(binary.BigEndian.Uint32(body[8:12]))
	oldSid := int(binary.BigEndian.Uint32(body[12:16]))
	newSid := int(binary.BigEndian.Uint32(body[16:20]))
	if slot != 0 || oldSid != 10001 || newSid != 10002 {
		t.Fatalf("slot=%d old=%d new=%d", slot, oldSid, newSid)
	}
}

func TestApplyLevelUpSkillsAutoAndReplace(t *testing.T) {
	cat := tableloader.New(testXMLDir(t))
	if err := cat.Load(); err != nil {
		t.Fatalf("catalog: %v", err)
	}
	s := &Server{cfg: Config{Catalog: cat}}
	p := &store.Pet{PetID: 1, Level: 5, CatchTime: 99, Skills: []int{10001, 20001}}
	p.Level = 13
	note := s.applyLevelUpSkills(p, 5)
	if note == nil {
		t.Fatal("expect 2507")
	}
	if p.Skills[2] != 10002 || p.Skills[3] != 10003 {
		t.Fatalf("auto skills=%v", p.Skills)
	}
	p.Level = 17
	note2 := s.applyLevelUpSkills(p, 13)
	if note2 == nil {
		t.Fatal("expect replace note")
	}
	activeN := binary.BigEndian.Uint32(note2[8:12])
	unN := binary.BigEndian.Uint32(note2[12:16])
	if activeN != 0 || unN != 1 {
		t.Fatalf("active=%d unactive=%d body=%v", activeN, unN, note2)
	}
	if binary.BigEndian.Uint32(note2[16:20]) != 20008 {
		t.Fatalf("unactive skill=%d", binary.BigEndian.Uint32(note2[16:20]))
	}
}

func TestPetCombatStatsFromCatalog(t *testing.T) {
	cat := tableloader.New(testXMLDir(t))
	if err := cat.Load(); err != nil {
		t.Fatalf("catalog: %v", err)
	}
	defaultSkillCatalog = cat
	defer func() { defaultSkillCatalog = nil }()
	p := &store.Pet{PetID: 58, Level: 10, DV: 20, Name: ""}
	_, _, name, hp, atk, _, _, _, _ := petCombatStats(p)
	if name != "塔奇拉顿" {
		t.Fatalf("name=%s", name)
	}
	wantHP := calcHP(100, 20, 10, 0)
	if hp != wantHP {
		t.Fatalf("hp=%d want=%d (catalog race values)", hp, wantHP)
	}
	if atk <= 0 {
		t.Fatal("atk")
	}
}
