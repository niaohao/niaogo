package tableloader

import (
	"path/filepath"
	"testing"
)

func TestPetBaseLearnableMoves(t *testing.T) {
	xmlDir := filepath.Join("..", "..", "tables", "xml")
	c := New(xmlDir)
	if err := c.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	b := c.PetBase(1)
	if b == nil || b.Name != "布布种子" || b.HP != 55 {
		t.Fatalf("pet1 base=%+v", b)
	}
	if b.YieldingExp <= 0 {
		t.Fatalf("pet1 YieldingExp=%d", b.YieldingExp)
	}
	if y := c.YieldingExpOf(1); y != b.YieldingExp {
		t.Fatalf("YieldingExpOf=%d want %d", y, b.YieldingExp)
	}
	if c.YieldingExpOf(0) != 0 {
		t.Fatal("missing pet should be 0")
	}
	if !c.CanLearnMove(1, 10002) {
		t.Fatal("布布种子应可学 10002")
	}
	at9 := c.SkillsLearnedAtLevel(1, 9)
	if len(at9) != 1 || at9[0] != 10002 {
		t.Fatalf("lv9 skills=%v", at9)
	}
	between := c.SkillsLearnedBetween(1, 5, 13)
	// 9→10002, 13→10003
	if len(between) < 2 {
		t.Fatalf("between 5-13=%v", between)
	}
}

func TestDefaultSkillsAtLevelHighToLow(t *testing.T) {
	xmlDir := filepath.Join("..", "..", "tables", "xml")
	c := New(xmlDir)
	if err := c.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	skills := c.DefaultSkillsAtLevel(1045, 100)
	if len(skills) != 4 {
		t.Fatalf("want 4 skills got %v", skills)
	}
	for _, id := range skills {
		if c.Skill(id) == nil {
			t.Fatalf("unknown skill %d in %v", id, skills)
		}
	}
	// 高等级优先：12316(61) 21278(57) 12455(53) 12453(49)（补表后 21278 可用）
	want := []int{12316, 21278, 12455, 12453}
	for i := range want {
		if skills[i] != want[i] {
			t.Fatalf("skills=%v want %v", skills, want)
		}
	}
}
