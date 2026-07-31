package gameserver

import (
	"encoding/binary"
	"testing"
	"time"

	"niaohao/server/internal/store"
)

func TestResolveLeiyiTrainBoss(t *testing.T) {
	pid, lv, name, ok := resolveLeiyiTrainBoss(10000)
	if !ok || pid != 70 || lv != 50 || name == "" {
		t.Fatalf("10000 got ok=%v pet=%d lv=%d name=%q", ok, pid, lv, name)
	}
	pid, lv, name, ok = resolveLeiyiTrainBoss(10006)
	if !ok || pid != 5005 || lv != 100 {
		t.Fatalf("10006 got ok=%v pet=%d lv=%d", ok, pid, lv)
	}
	if _, _, _, ok := resolveLeiyiTrainBoss(9999); ok {
		t.Fatal("9999 should miss")
	}
	pid2, _, name2 := resolveChallengeBoss(32, 10003)
	if pid2 != 70 || name2 != "雷伊幻影" {
		t.Fatalf("challenge 10003 got pet=%d name=%q", pid2, name2)
	}
}

func TestNormalizeLeiyiTrain(t *testing.T) {
	st, changed := store.NormalizeLeiyiTrain(store.LeiyiTrainProgress{}, time.Unix(1_700_000_000, 0))
	if !changed || len(st.Total) != 6 || st.Total[0] != 60 || st.Total[1] != 30 {
		t.Fatalf("defaults total=%v changed=%v", st.Total, changed)
	}
	st2, _ := store.NormalizeLeiyiTrain(st, time.Unix(1_700_000_000+86400, 0))
	for _, v := range st2.Today {
		if v != 0 {
			t.Fatalf("today should reset: %v", st2.Today)
		}
	}
}

func TestBuildGaiyaEffectInfoBody(t *testing.T) {
	b := buildGaiyaEffectInfoBody(0, 0)
	if len(b) != 8 || binary.BigEndian.Uint32(b[0:4]) != 0 {
		t.Fatalf("empty body=%v", b)
	}
	b = buildGaiyaEffectInfoBody(1, 0b111)
	if binary.BigEndian.Uint32(b[0:4]) != 1 {
		t.Fatalf("def=%d", binary.BigEndian.Uint32(b[0:4]))
	}
	// compat: count+1 then real count
	if binary.BigEndian.Uint32(b[4:8]) != 4 {
		t.Fatalf("compatCount=%d", binary.BigEndian.Uint32(b[4:8]))
	}
	if binary.BigEndian.Uint32(b[8:12]) != 3 {
		t.Fatalf("realCount=%d", binary.BigEndian.Uint32(b[8:12]))
	}
}

func TestPetSixStatsAppliesTrainBonus(t *testing.T) {
	p := &store.Pet{PetID: 70, Level: 50, DV: 31, Nature: 0}
	baseHP, _, _, _, _, _ := petSixStatsFromPet(p)
	p.Bonus[0] = 10
	hp, _, _, _, _, _ := petSixStatsFromPet(p)
	if hp != baseHP+10 {
		t.Fatalf("hp=%d want %d", hp, baseHP+10)
	}
}

func TestLeiyiTrainRewardSkillForTaskStep(t *testing.T) {
	if leiyiTrainRewardSkillForTaskStep(121, 0) != 10823 {
		t.Fatal("121")
	}
	if leiyiTrainRewardSkillForTaskStep(122, 3) != 10825 {
		t.Fatal("122.3")
	}
}
