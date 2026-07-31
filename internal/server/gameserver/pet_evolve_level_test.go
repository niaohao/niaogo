package gameserver

import (
	"path/filepath"
	"testing"

	"niaohao/server/internal/store"
	"niaohao/server/internal/tableloader"
)

func TestDirectEvolveOnLevel(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "tables", "xml")
	cat := tableloader.New(dir)
	if err := cat.Load(); err != nil {
		t.Fatal(err)
	}
	defaultSkillCatalog = cat
	defer func() { defaultSkillCatalog = nil }()

	s := &Server{cfg: Config{Catalog: cat}}
	p := &store.Pet{PetID: 1, Name: "布布种子", Level: 17, Exp: 0, Skills: []int{10001}}
	need := petNextLevelExp(1, 17)
	_ = applyPetExpGain(p, need) // -> 18
	if p.Level < 18 {
		t.Fatalf("lv=%d want >=18", p.Level)
	}
	note := s.afterPetLevelChange(p, 17)
	_ = note
	if p.PetID != 2 {
		t.Fatalf("petID=%d want 2 (布布草)", p.PetID)
	}
	if p.Name == "布布种子" {
		t.Fatalf("name should update, got %q", p.Name)
	}
}

func TestDirectEvolveSkipsBabin(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "tables", "xml")
	cat := tableloader.New(dir)
	if err := cat.Load(); err != nil {
		t.Fatal(err)
	}
	// 找一只 EvolveBabin=1 的精灵
	var babinID int
	for id := 1; id < 500; id++ {
		d := cat.PetBase(id)
		if d != nil && d.EvolveBabin == 1 && d.EvolvesTo > 0 {
			babinID = id
			break
		}
	}
	if babinID == 0 {
		t.Skip("no EvolveBabin pet in sample")
	}
	d := cat.PetBase(babinID)
	p := &store.Pet{PetID: babinID, Level: d.EvolvingLv + 10}
	if tryDirectEvolvePet(cat, p) {
		t.Fatalf("EvolveBabin should not auto-evolve, pet %d", babinID)
	}
}

func TestAdjustSkillPowerEffects(t *testing.T) {
	d2 := &tableloader.SkillDef{SideEffect: "2", Power: 65}
	if got := adjustSkillPower(d2, 65, skillPowerAdj{FoeHP: 40, FoeMaxHP: 100}); got != 130 {
		t.Fatalf("effect2 half hp: %d", got)
	}
	if got := adjustSkillPower(d2, 65, skillPowerAdj{FoeHP: 60, FoeMaxHP: 100}); got != 65 {
		t.Fatalf("effect2 full: %d", got)
	}

	d30 := &tableloader.SkillDef{SideEffect: "30", Power: 45}
	if got := adjustSkillPower(d30, 45, skillPowerAdj{GoingFirst: false}); got != 90 {
		t.Fatalf("effect30: %d", got)
	}
	d40 := &tableloader.SkillDef{SideEffect: "40", Power: 70}
	if got := adjustSkillPower(d40, 70, skillPowerAdj{GoingFirst: true}); got != 140 {
		t.Fatalf("effect40: %d", got)
	}

	d9 := &tableloader.SkillDef{SideEffect: "9", SideEffectArg: "20 80", Power: 40}
	if got := adjustSkillPower(d9, 40, skillPowerAdj{ConsecCount: 2}); got != 80 {
		t.Fatalf("effect9: %d", got)
	}
	if got := adjustSkillPower(d9, 40, skillPowerAdj{ConsecCount: 10}); got != 120 {
		t.Fatalf("effect9 cap: %d", got)
	}

	stages := [5]int8{2, 1, 0, 0, 0}
	d35 := &tableloader.SkillDef{SideEffect: "35", Power: 60}
	if got := adjustSkillPower(d35, 60, skillPowerAdj{FoeStages: &stages}); got != 120 {
		t.Fatalf("effect35: %d", got)
	}
}

func TestSameLifeDamage(t *testing.T) {
	d := &tableloader.SkillDef{SideEffect: "7", Power: 0}
	dmg, ok := sameLifeDamage(d, 30, 80)
	if !ok || dmg != 50 {
		t.Fatalf("dmg=%d ok=%v", dmg, ok)
	}
	dmg, ok = sameLifeDamage(d, 50, 40)
	if !ok || dmg != 0 {
		t.Fatalf("should miss: dmg=%d ok=%v", dmg, ok)
	}
}

func TestSideEffectBuffBatch(t *testing.T) {
	d28 := &tableloader.SkillDef{SideEffect: "28", SideEffectArg: "4"}
	if sideEffectPercentHPDamage(d28, 100) != 25 {
		t.Fatal("28")
	}
	d36 := &tableloader.SkillDef{SideEffect: "36", SideEffectArg: "100"}
	if sideEffectOHKO(d36, 80) != 80 {
		t.Fatal("36")
	}
	d37 := &tableloader.SkillDef{SideEffect: "37", SideEffectArg: "3 2", Power: 60}
	if adjustSkillPowerSelfHP(d37, 60, 20, 100) != 120 {
		t.Fatal("37 low")
	}
	if adjustSkillPowerSelfHP(d37, 60, 50, 100) != 60 {
		t.Fatal("37 high")
	}
	st := &BattleState{PlayerMaxHP: 100, PlayerHP: 100, EnemyMaxHP: 100, EnemyHP: 100, EnemyType: 8}
	argOff := applyOneOngoingBuff(st, 48, []int{3}, 0, true)
	if st.PlayerBuff.ImmuneStatusRounds != 3 || argOff != 1 {
		t.Fatalf("48 rounds=%d off=%d", st.PlayerBuff.ImmuneStatusRounds, argOff)
	}
	applyOneOngoingBuff(st, 13, []int{5}, 0, true)
	if st.EnemyBuff.DrainRounds != 5 {
		t.Fatal("13")
	}
	st.EnemyBuff.DrainRounds = 1
	st.PlayerHP = 50
	tickBattleBuffs(st)
	if st.EnemyHP != 88 || st.PlayerHP != 62 {
		t.Fatalf("drain tick enemy=%d player=%d", st.EnemyHP, st.PlayerHP)
	}
	def := &battleBuff{ImmuneHits: 1}
	if applyIncomingDamageBuff(def, &tableloader.SkillDef{Category: 1}, 50) != 0 || def.ImmuneHits != 0 {
		t.Fatal("46")
	}
}

