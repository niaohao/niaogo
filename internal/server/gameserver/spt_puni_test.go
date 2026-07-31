package gameserver

import (
	"testing"

	"niaohao/server/internal/tableloader"
)

func TestPuniVoidForcesMiss(t *testing.T) {
	st := &BattleState{
		EnemyID: petIDPuni, MapID: 514, BossRegion: puniRegionVoid,
		EnemyHP: 5000, EnemyMaxHP: 5000,
	}
	d := &tableloader.SkillDef{MustHit: 0, Category: 1, Type: 8}
	dmg, hit := applyPuniOnPlayerSkillHit(st, 10001, d, 500, true)
	if hit || dmg != 0 {
		t.Fatalf("void should miss non-musthit: hit=%v dmg=%d", hit, dmg)
	}
	d.MustHit = 1
	dmg, hit = applyPuniOnPlayerSkillHit(st, 10001, d, 500, true)
	if !hit || dmg != 500 {
		t.Fatalf("void allows musthit: hit=%v dmg=%d", hit, dmg)
	}
}

func TestPuniElementAlternate(t *testing.T) {
	st := &BattleState{
		EnemyID: petIDPuni, MapID: 514, BossRegion: puniRegionElement,
		EnemyHP: 6000, EnemyMaxHP: 6000,
	}
	fire := &tableloader.SkillDef{Category: 1, Type: 3}
	dmg, _ := applyPuniOnPlayerSkillHit(st, 1, fire, 200, true)
	if dmg != 0 {
		t.Fatal("non light/dark should deal 0")
	}
	light := &tableloader.SkillDef{Category: 1, Type: 12}
	dmg, _ = applyPuniOnPlayerSkillHit(st, 2, light, 200, true)
	if dmg != 200 || st.PuniElementLastType != 12 {
		t.Fatalf("light ok dmg=%d last=%d", dmg, st.PuniElementLastType)
	}
	dmg, _ = applyPuniOnPlayerSkillHit(st, 2, light, 200, true)
	if dmg != 0 {
		t.Fatal("same element twice should 0")
	}
	dark := &tableloader.SkillDef{Category: 1, Type: 13}
	dmg, _ = applyPuniOnPlayerSkillHit(st, 3, dark, 200, true)
	if dmg != 200 {
		t.Fatal("alternate dark should work")
	}
}

func TestPuniEnergyBacklash(t *testing.T) {
	st := &BattleState{
		EnemyID: petIDPuni, MapID: 514, BossRegion: puniRegionEnergy,
		EnemyHP: 7000, EnemyMaxHP: 7000, PlayerHP: 1000, PlayerMaxHP: 1000,
	}
	d := &tableloader.SkillDef{Category: 1, Type: 8}
	dmg, hit := applyPuniOnPlayerSkillHit(st, 1, d, 250, true)
	if !hit || dmg != 100 || st.PlayerHP != 0 {
		t.Fatalf("energy: dmg=%d hit=%v php=%d", dmg, hit, st.PlayerHP)
	}
}

func TestPuniEternalHalf(t *testing.T) {
	st := &BattleState{EnemyID: petIDPuni, MapID: 514, BossRegion: puniRegionEternal, EnemyHP: 100}
	if got := applyPuniEternalHalf(st, 100); got != 50 {
		t.Fatal(got)
	}
	st.BossRegion = puniRegionHoly
	if got := applyPuniEternalHalf(st, 3); got != 1 {
		t.Fatal(got)
	}
}

func TestPuniSamsaraLifeSwitch(t *testing.T) {
	st := &BattleState{
		EnemyID: petIDPuni, MapID: 514, BossRegion: puniRegionSamsara,
		EnemyHP: 0, EnemyMaxHP: 10000,
		PuniTotalLives: 2, PuniCurrentLife: 1,
		EnemyStages: [5]int8{3, 0, 0, 0, 0},
	}
	if !tryPuniLifeSwitch(st) {
		t.Fatal("should switch")
	}
	if st.PuniCurrentLife != 2 || st.EnemyHP != 10000 || st.EnemyStages[0] != 2 {
		t.Fatalf("cur=%d hp=%d atk=%d", st.PuniCurrentLife, st.EnemyHP, st.EnemyStages[0])
	}
	st.EnemyHP = 0
	if tryPuniLifeSwitch(st) {
		t.Fatal("second death should not switch")
	}
}

func TestPuniTrueFormSixLives(t *testing.T) {
	st := &BattleState{
		EnemyID: petIDPuni, MapID: 514, BossRegion: puniRegionTrue,
		EnemyHP: 0, EnemyMaxHP: 7000,
		PuniTotalLives: 6, PuniCurrentLife: 1,
	}
	if !tryPuniLifeSwitch(st) {
		t.Fatal("life1->2")
	}
	if st.PuniCurrentLife != 2 || st.EnemyMaxHP != 8000 {
		t.Fatalf("life=%d max=%d", st.PuniCurrentLife, st.EnemyMaxHP)
	}
}

func TestPuniLifeRegen(t *testing.T) {
	st := &BattleState{
		EnemyID: petIDPuni, MapID: 514, BossRegion: puniRegionLife,
		EnemyHP: 1000, EnemyMaxHP: 8000,
	}
	applyPuniRoundStart(st)
	if st.EnemyHP != 3000 {
		t.Fatal(st.EnemyHP)
	}
}

func TestPuniControlVulnerable(t *testing.T) {
	st := &BattleState{EnemyID: petIDPuni, MapID: 514, BossRegion: puniRegionElement}
	if !canApplyEnemyBattleStatus(st, 10) {
		t.Fatal("element should allow para")
	}
	st.BossRegion = puniRegionVoid
	if canApplyEnemyBattleStatus(st, 10) {
		t.Fatal("void should block control via listed boss immune")
	}
}

func TestPuniTimeSenseSuppress(t *testing.T) {
	st := &BattleState{
		EnemyID: petIDPuni, MapID: 514, BossRegion: puniRegionTrue,
		PuniTotalLives: 6, PuniCurrentLife: 1,
		EnemyHP: 7000, EnemyMaxHP: 7000,
	}
	d := &tableloader.SkillDef{MustHit: 1, Category: 4, Type: 8}
	_, _ = applyPuniOnPlayerSkillHit(st, puniTimeSenseSkillID, d, 0, true)
	if !st.PuniTrueFormSuppressed {
		t.Fatal("20300 should suppress void")
	}
	atk := &tableloader.SkillDef{MustHit: 0, Category: 1, Type: 8}
	dmg, hit := applyPuniOnPlayerSkillHit(st, 10001, atk, 100, true)
	if !hit || dmg != 100 {
		t.Fatalf("after suppress void should not block: hit=%v dmg=%d", hit, dmg)
	}
}

func TestPuniEnemyPPInfinite(t *testing.T) {
	st := &BattleState{EnemyID: petIDPuni, MapID: 514, BossRegion: puniRegionEternal}
	if !enemyHasInfinitePP(st) {
		t.Fatal("eternal")
	}
	st.BossRegion = puniRegionVoid
	if enemyHasInfinitePP(st) {
		t.Fatal("void no infinite")
	}
}
