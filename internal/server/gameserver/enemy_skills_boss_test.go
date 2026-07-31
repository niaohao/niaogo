package gameserver

import (
	"testing"
)

func TestEnemySkillsBossNotOnlyTackle(t *testing.T) {
	cat := testCatalog(t)
	old := defaultSkillCatalog
	defaultSkillCatalog = cat
	defer func() { defaultSkillCatalog = old }()

	s := &Server{cfg: Config{Catalog: cat}}
	// 蘑菇怪 Lv10：可学含撞击+飞叶快刀；不应只会 10001
	sk := s.enemySkillsForPet(47, 10)
	if len(sk) < 2 {
		t.Fatalf("mushroom want >=2 skills, got %v", sk)
	}
	hasNonTackle := false
	for _, p := range sk {
		if p[0] != 0 && p[0] != 10001 {
			hasNonTackle = true
		}
	}
	if !hasNonTackle {
		t.Fatalf("mushroom skills should include non-tackle: %v", sk)
	}

	st := &BattleState{EnemyID: 47, EnemySkills: sk}
	seen := map[uint32]bool{}
	for i := 0; i < 40; i++ {
		seen[s.pickEnemyBattleSkill(st)] = true
	}
	if len(seen) < 2 {
		t.Fatalf("pick should vary among skills, got %v from %v", seen, sk)
	}

	// 雷伊高等级应有强力技，而非仅撞击
	lei := s.enemySkillsForPet(70, 70)
	ids := make([]uint32, 0, len(lei))
	for _, p := range lei {
		ids = append(ids, p[0])
	}
	if len(ids) == 0 || (len(ids) == 1 && ids[0] == 10001) {
		t.Fatalf("leiyi skills=%v", ids)
	}
}
