package gameserver

import "testing"

func TestCollectSPTAchieveHitsClassic(t *testing.T) {
	hits := collectSPTAchieveHits(&BattleState{EnemyID: 47})
	if len(hits) != 1 || hits[0].BranchID != 2 || hits[0].Threshold != 301 {
		t.Fatalf("mushroom: %+v", hits)
	}
	hits = collectSPTAchieveHits(&BattleState{EnemyID: 70})
	if len(hits) != 1 || hits[0].Threshold != 306 {
		t.Fatalf("ray: %+v", hits)
	}
}

func TestCollectSPTAchieveHitsExpanded(t *testing.T) {
	for pet, th := range map[int]int{421: 316, 490: 317, 587: 320, 715: 323} {
		hits := collectSPTAchieveHits(&BattleState{EnemyID: pet})
		if len(hits) != 1 || hits[0].Threshold != th {
			t.Fatalf("pet=%d got %+v want th=%d", pet, hits, th)
		}
	}
}

func TestCollectSPTAchieveHitsKraniteHard(t *testing.T) {
	hits := collectSPTAchieveHits(&BattleState{EnemyID: 538, MapID: 435, BossRegion: 2})
	found318, found319 := false, false
	for _, h := range hits {
		if h.Threshold == 318 {
			found318 = true
		}
		if h.Threshold == 319 {
			found319 = true
		}
	}
	if !found318 || !found319 {
		t.Fatalf("hard kranite: %+v", hits)
	}
}

func TestCollectSPTAchieveHitsFourBeast(t *testing.T) {
	hits := collectSPTAchieveHits(&BattleState{EnemyID: 501})
	ok := false
	for _, h := range hits {
		if h.BranchID == 21 && h.Threshold == 145 {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("bast: %+v", hits)
	}
}

func TestCollectSPTAchieveHitsPuniTrue(t *testing.T) {
	hits := collectSPTAchieveHits(&BattleState{EnemyID: 300, MapID: 514, BossRegion: 8})
	ok := false
	for _, h := range hits {
		if h.BranchID == 31 && h.Threshold == 1 {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("puni: %+v", hits)
	}
	// 封印非真身不给 Branch31
	hits = collectSPTAchieveHits(&BattleState{EnemyID: 300, MapID: 514, BossRegion: 1})
	for _, h := range hits {
		if h.BranchID == 31 {
			t.Fatal("seal should not grant branch31")
		}
	}
}
