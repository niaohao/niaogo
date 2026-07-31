package gameserver

import (
	"encoding/binary"
	"path/filepath"
	"testing"

	"niaohao/server/internal/store"
	"niaohao/server/internal/tableloader"
)

func TestApplyPetExpGainLevels(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "tables", "xml")
	cat := tableloader.New(dir)
	if err := cat.Load(); err != nil {
		t.Fatal(err)
	}
	defaultSkillCatalog = cat
	defer func() { defaultSkillCatalog = nil }()

	p := &store.Pet{PetID: 1, Level: 1, Exp: 0}
	need1 := petNextLevelExp(1, 1)
	if need1 < 1 {
		t.Fatalf("need1=%d", need1)
	}
	used := applyPetExpGain(p, need1+need1/2)
	if used != need1+need1/2 {
		t.Fatalf("used=%d want=%d", used, need1+need1/2)
	}
	if p.Level != 2 {
		t.Fatalf("lv=%d want 2", p.Level)
	}
}

func TestApplyPetExpGainCap100(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "tables", "xml")
	cat := tableloader.New(dir)
	_ = cat.Load()
	defaultSkillCatalog = cat
	defer func() { defaultSkillCatalog = nil }()

	p := &store.Pet{PetID: 1, Level: 99, Exp: 0}
	need := petNextLevelExp(1, 99)
	used := applyPetExpGain(p, need+1000)
	if used != need {
		t.Fatalf("used=%d want %d", used, need)
	}
	if p.Level != 100 || p.Exp != 0 {
		t.Fatalf("lv=%d exp=%d", p.Level, p.Exp)
	}
	if applyPetExpGain(p, 50) != 0 {
		t.Fatal("lv100 should not consume")
	}
}

func TestFillPetSkillsUpToFour(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "tables", "xml")
	cat := tableloader.New(dir)
	if err := cat.Load(); err != nil {
		t.Fatal(err)
	}
	defaultSkillCatalog = cat
	defer func() { defaultSkillCatalog = nil }()
	s := &Server{cfg: Config{Catalog: cat}}
	p := &store.Pet{PetID: 1, Level: 20, Skills: []int{10001}}
	if !s.fillPetSkillsUpToFour(p) {
		t.Fatal("expected fill")
	}
	n := 0
	for _, sid := range p.Skills {
		if sid > 0 {
			n++
		}
	}
	if n < 2 {
		t.Fatalf("skills=%v count=%d", p.Skills, n)
	}
}

func TestExeListBodyLayout(t *testing.T) {
	out := make([]byte, 4+20)
	binary.BigEndian.PutUint32(out[0:4], 1)
	binary.BigEndian.PutUint32(out[4:8], 0)
	binary.BigEndian.PutUint32(out[8:12], 1768449796)
	binary.BigEndian.PutUint32(out[12:16], 4)
	binary.BigEndian.PutUint32(out[16:20], 86400)
	binary.BigEndian.PutUint32(out[20:24], 1)
	if binary.BigEndian.Uint32(out[16:20])/3600 != 24 {
		t.Fatal("remain hours")
	}
}
