package gameserver

import "testing"

func TestEnemySkillsIncarose(t *testing.T) {
	cat := testCatalog(t)
	s := &Server{cfg: Config{Catalog: cat}}
	ids := s.enemyDefaultSkillIDs(124, 100)
	if len(ids) == 0 || (len(ids) == 1 && ids[0] == 10001) {
		t.Fatalf("incarose default=%v", ids)
	}
	for _, id := range ids {
		if id == 26026 || id == 31836 || id == 26027 || id == 31837 {
			t.Fatalf("should skip missing skill table ids, got %v", ids)
		}
		if !s.skillIDKnown(id) {
			t.Fatalf("unknown skill %d in %v", id, ids)
		}
	}
	sk := s.enemySkillsForPet(124, 100)
	hasNonTackle := false
	for _, p := range sk {
		if p[0] != 0 && p[0] != 10001 {
			hasNonTackle = true
		}
	}
	if !hasNonTackle {
		t.Fatalf("incarose should not only tackle: default=%v forPet=%v", ids, sk)
	}
	// 高等级应带上强力斩系（表内）
	wantAny := map[uint32]bool{10369: true, 10372: true, 10368: true, 10371: true}
	ok := false
	for _, p := range sk {
		if wantAny[p[0]] {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("expect strong moves among %v, got %v", wantAny, sk)
	}
}
