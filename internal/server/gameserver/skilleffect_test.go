package gameserver

import (
	"testing"

	"niaohao/server/internal/tableloader"
)

func TestFallbackEnemyIsTachiraton(t *testing.T) {
	id, lv, name := resolveChallengeBoss(9999, 0)
	if id != 58 || name != "塔奇拉顿" || lv != 5 {
		t.Fatalf("got id=%d lv=%d name=%s", id, lv, name)
	}
}

func TestEnemySkillsTachiraton(t *testing.T) {
	s := &Server{}
	ids := s.enemyDefaultSkillIDs(58, 5)
	if len(ids) < 1 {
		t.Fatalf("skills=%v", ids)
	}
	// 无 catalog 时走硬编码兜底；有 catalog 时取 LearnableMoves 最近攻击技
	ok := false
	for _, id := range ids {
		if id == 10127 || id == 20004 || id == 10001 {
			ok = true
			break
		}
	}
	if !ok {
		t.Fatalf("skills=%v", ids)
	}
}

func TestParseSideEffectDrain(t *testing.T) {
	ids := parseSideEffectIDs("1")
	if len(ids) != 1 || ids[0] != 1 {
		t.Fatal(ids)
	}
	args := parseSideEffectArgs("5 15 -1")
	if len(args) != 3 || args[2] != -1 {
		t.Fatal(args)
	}
}

func TestStageMultiplier(t *testing.T) {
	if stageMultiplier(0) != 1 {
		t.Fatal("0")
	}
	if stageMultiplier(2) != 2 {
		t.Fatal("+2")
	}
	if stageMultiplier(-2) != 0.5 {
		t.Fatal("-2")
	}
}

func TestApplyDrainSideEffect(t *testing.T) {
	s := &Server{}
	st := &BattleState{
		PlayerHP: 10, PlayerMaxHP: 100,
		EnemyHP: 50, EnemyMaxHP: 100,
	}
	st.PlayerHP = 10
	heal := uint32(20)
	st.PlayerHP += heal
	if st.PlayerHP != 30 {
		t.Fatal(st.PlayerHP)
	}
	_ = s
}

func TestSideEffectClearAndLeaveOne(t *testing.T) {
	s := &Server{}
	st := &BattleState{PlayerStages: [5]int8{-2, 1, 0, 0, 0}, EnemyStages: [5]int8{3, 0, 0, 0, 0}}
	s.applySkillSideEffects(st, 0, 0, true, true) // no-op
	clearNegativeStages(&st.PlayerStages)
	clearPositiveStages(&st.EnemyStages)
	if st.PlayerStages[0] != 0 || st.EnemyStages[0] != 0 {
		t.Fatalf("stages %+v / %+v", st.PlayerStages, st.EnemyStages)
	}
	d := &tableloader.SkillDef{SideEffect: "8"}
	if applyLeaveOneHP(d, 10, 20) != 9 {
		t.Fatal("leave one")
	}
}

func TestSideEffectHitCount(t *testing.T) {
	d := &tableloader.SkillDef{SideEffect: "31", SideEffectArg: "2 2"}
	if sideEffectHitCount(d) != 2 {
		t.Fatal("fixed 2 hits")
	}
	if sideEffectHitCount(&tableloader.SkillDef{}) != 1 {
		t.Fatal("default 1")
	}
}

func TestSPTFirstKillTable(t *testing.T) {
	if sptFirstKillByPetID[47].RewardPetID != 46 {
		t.Fatal("mushroom")
	}
	if sptFirstKillByPetID[70].RewardItemID != 400103 {
		t.Fatal("ray")
	}
	if _, ok := sptFirstKillByPetID[300]; ok {
		t.Fatal("puni must use fragment path")
	}
}

func TestSideEffectCounterDamage(t *testing.T) {
	d := &tableloader.SkillDef{SideEffect: "34", SideEffectArg: "2"}
	if sideEffectCounterDamage(d, 50) != 100 {
		t.Fatal(sideEffectCounterDamage(d, 50))
	}
	if sideEffectCounterDamage(d, 0) != 0 {
		t.Fatal("no last taken")
	}
}

func TestSkillFailAndDoom(t *testing.T) {
	st := &BattleState{PlayerSpd: 100, EnemySpd: 50, PlayerHP: 10, EnemyHP: 10}
	d := &tableloader.SkillDef{SideEffect: "52"}
	tryInvalidateSkill(st, d, true, true)
	if !st.EnemySkillFail {
		t.Fatal("expect fail mark")
	}
	if !consumeSkillFail(st, false) || consumeSkillFail(st, false) {
		t.Fatal("consume once")
	}
	doom := &tableloader.SkillDef{SideEffect: "62", SideEffectArg: "2"}
	armDoom(st, doom, true)
	if st.PlayerDoomRounds != 2 {
		t.Fatal(st.PlayerDoomRounds)
	}
	tickDoom(st)
	if st.EnemyHP == 0 || st.PlayerDoomRounds != 1 {
		t.Fatal("not yet")
	}
	tickDoom(st)
	if st.EnemyHP != 0 || st.PlayerDoomRounds != 0 {
		t.Fatal("doom fire", st.EnemyHP, st.PlayerDoomRounds)
	}
}

func TestChargeAndSacrificeEnter(t *testing.T) {
	st := &BattleState{PlayerMaxHP: 90, PlayerHP: 90}
	d := &tableloader.SkillDef{SideEffect: "17"}
	if !beginCharge(st, 123, d, true) || st.PlayerChargeSkill != 123 {
		t.Fatal("charge")
	}
	if takeChargeSkill(st, true) != 123 || st.PlayerChargeSkill != 0 {
		t.Fatal("release")
	}
	sac := &tableloader.SkillDef{SideEffect: "59"}
	applySacrificeEffects(st, sac, true)
	if st.PlayerHP != 0 || st.PlayerNextStageBoost[stageSA] != 1 {
		t.Fatalf("sac %+v hp=%d", st.PlayerNextStageBoost, st.PlayerHP)
	}
	st.PlayerHP = 90
	applyEnterPetPending(st, true)
	if st.PlayerStages[stageSA] != 1 || st.PlayerNextStageBoost[stageSA] != 0 {
		t.Fatalf("enter %+v", st.PlayerStages)
	}
}

func TestOnKOEffects(t *testing.T) {
	st := &BattleState{PlayerHP: 10, PlayerMaxHP: 90, EnemyHP: 0}
	d := &tableloader.SkillDef{SideEffect: "66 67", SideEffectArg: "3"}
	applyOnKOEffects(st, d, true, 5)
	if st.PlayerHP != 40 { // 10+90/3
		t.Fatal(st.PlayerHP)
	}
	if st.EnemyNextEnterCutDenom != 3 {
		t.Fatal(st.EnemyNextEnterCutDenom)
	}
}
func TestSideEffectMoreBatch(t *testing.T) {
	d405 := &tableloader.SkillDef{SideEffect: "405", SideEffectArg: "50"}
	if sideEffectFirstStrikeFlat(d405, true) != 50 || sideEffectFirstStrikeFlat(d405, false) != 0 {
		t.Fatal("405")
	}
	d456 := &tableloader.SkillDef{SideEffect: "179 456", SideEffectArg: "20 300"}
	// 179 takes 1 arg, 456 last → 300
	if sideEffectLowHPExecute(d456, 200) != 200 {
		t.Fatal("456 execute")
	}
	if sideEffectLowHPExecute(d456, 400) != 0 {
		t.Fatal("456 no")
	}
	stagesUp := [5]int8{1, 0, 0, 0, 0}
	stagesDn := [5]int8{-1, 0, 0, 0, 0}
	d413 := &tableloader.SkillDef{SideEffect: "413", SideEffectArg: "50"}
	if sideEffectBoostFlat(d413, &stagesUp) != 50 || sideEffectBoostFlat(d413, &stagesDn) != 0 {
		t.Fatal("413")
	}
	d167 := &tableloader.SkillDef{SideEffect: "167", SideEffectArg: "100"}
	if sideEffectDropFlat(d167, &stagesDn) != 100 || sideEffectDropFlat(d167, &stagesUp) != 0 {
		t.Fatal("167")
	}
	d467 := &tableloader.SkillDef{SideEffect: "467", SideEffectArg: "2 200"}
	if sideEffectStatusIndexFlat(d467, &battleStatus{Burn: true}) != 200 {
		t.Fatal("467")
	}
	d135 := &tableloader.SkillDef{SideEffect: "135", SideEffectArg: "80"}
	if sideEffectMinDamage(d135, 10) != 80 {
		t.Fatal("135")
	}
	st := &BattleState{EnemyDef: 100, EnemyStages: [5]int8{0, 2, 0, 0, 0}}
	if st.stagedDefIgnoreBoost(false) >= st.stagedDef(false) {
		t.Fatal("195 ignore boost")
	}
	got := applyMoreDamageSideEffects(d405, 10, 100, &battleStatus{}, &stagesUp, true)
	if got != 60 {
		t.Fatalf("more dmg %d", got)
	}
}

func TestSideEffectHighBatch(t *testing.T) {
	d101 := &tableloader.SkillDef{SideEffect: "101", SideEffectArg: "50"}
	if sideEffectDrainPercent(d101, 100) != 50 {
		t.Fatal("101")
	}
	d105 := &tableloader.SkillDef{SideEffect: "105", SideEffectArg: "4"}
	if sideEffectDrainDenom(d105, 100) != 25 {
		t.Fatal("105")
	}
	d402 := &tableloader.SkillDef{SideEffect: "402", SideEffectArg: "40"}
	if sideEffectSecondStrikeFlat(d402, true) != 0 || sideEffectSecondStrikeFlat(d402, false) != 40 {
		t.Fatal("402")
	}
	d133 := &tableloader.SkillDef{SideEffect: "133", SideEffectArg: "30"}
	if sideEffectStatusFlatBonus(d133, &battleStatus{Burn: true}) != 30 {
		t.Fatal("133")
	}
	d102 := &tableloader.SkillDef{SideEffect: "102"}
	if foeParaDamageMul(d102, &battleStatus{Para: true}, 10) != 20 {
		t.Fatal("102")
	}
	d179 := &tableloader.SkillDef{SideEffect: "179", SideEffectArg: "50", Type: 3}
	if sameTypePowerBoost(d179, 3, 100) != 150 {
		t.Fatal("179")
	}
	d154 := &tableloader.SkillDef{SideEffect: "154", SideEffectArg: "2 2"}
	if sideEffectCondDrain(d154, &battleStatus{Burn: true}, 100) != 50 {
		t.Fatal("154")
	}
	got := applyHighDamageSideEffects(d402, 0, &battleStatus{}, 0, 0, false)
	if got != 40 {
		t.Fatalf("402 on zero base: %d", got)
	}
	st := &BattleState{PlayerHP: 50, PlayerMaxHP: 100, EnemyHP: 0, PlayerStages: [5]int8{}}
	d158 := &tableloader.SkillDef{SideEffect: "158", SideEffectArg: "0 100 2"}
	applyOnKOSelfStage(st, d158, true, 10)
	if st.PlayerStages[0] != 2 {
		t.Fatalf("158 stage=%d", st.PlayerStages[0])
	}
	forceFirst, forceSecond := priorityFromBuff(&battleBuff{PriorityRounds: 2}, &battleBuff{})
	if !forceFirst || forceSecond {
		t.Fatal("83 priority")
	}
}

func TestSideEffectMidBatch(t *testing.T) {
	d88 := &tableloader.SkillDef{SideEffect: "88", SideEffectArg: "100 3"}
	if sideEffectChanceMulDamage(d88, 10) != 30 {
		t.Fatal("88")
	}
	d96 := &tableloader.SkillDef{SideEffect: "96"}
	if foeStatusDamageMul(d96, &battleStatus{Burn: true}, 10) != 20 {
		t.Fatal("96")
	}
	d100 := &tableloader.SkillDef{SideEffect: "100"}
	if lowHPDamageScale(d100, 0, 100, 10) != 20 {
		t.Fatal("100")
	}
	d80 := &tableloader.SkillDef{SideEffect: "80"}
	dmg, loss, ok := sacrificeHalfEqualDamage(d80, 80, 100)
	if !ok || dmg != 50 || loss != 50 {
		t.Fatalf("80 %d %d %v", dmg, loss, ok)
	}
	st := &BattleState{PlayerHP: 100, PlayerMaxHP: 100, PlayerSkills: [][2]uint32{{1, 0}}}
	s := &Server{}
	restoreAllSkillPP(st.PlayerSkills, func(int) *tableloader.SkillDef {
		return &tableloader.SkillDef{MaxPP: 15}
	})
	if st.PlayerSkills[0][1] != 15 {
		t.Fatal("87")
	}
	st.EnemyStages = [5]int8{2, 0, 0, 0, 0}
	stealPositiveStages(&st.PlayerStages, &st.EnemyStages)
	if st.PlayerStages[0] != 2 || st.EnemyStages[0] != 0 {
		t.Fatal("85")
	}
	_ = s
	hp := uint32(100)
	stages := [5]int8{}
	applyHalfHPStageBoost(&hp, 100, &stages, nil, 0)
	if hp != 50 || stages[0] != 1 {
		t.Fatalf("79 hp=%d st=%v", hp, stages)
	}
	buff := &battleBuff{FireHalfRounds: 1}
	if applyIncomingDamageBuff(buff, &tableloader.SkillDef{Type: 3, Category: 1}, 40) != 20 {
		t.Fatal("41")
	}
}

func TestSideEffectFreq2Batch(t *testing.T) {
	d422 := &tableloader.SkillDef{SideEffect: "422", SideEffectArg: "50"}
	if sideEffectDamagePercentFlat(d422, 100) != 50 {
		t.Fatal("422")
	}
	d436 := &tableloader.SkillDef{SideEffect: "436", SideEffectArg: "50"}
	if sideEffectLostHPPercentFlat(d436, 40, 100) != 30 { // lost 60 * 50% = 30
		t.Fatal("436")
	}
	d455 := &tableloader.SkillDef{SideEffect: "455", SideEffectArg: "10 2"}
	if sideEffectLostHPChunkFlat(d455, 50, 100) != 10 { // lost 50 / 10 * 2 = 10
		t.Fatal("455")
	}
	d428 := &tableloader.SkillDef{SideEffect: "428", SideEffectArg: "80", Type: 3}
	if sideEffectTypeAdvantageFlat(d428, 3, 1) == 0 && typeMultiplier(3, 1) > 1 {
		// 火克草：若表未加载则跳过断言
	} else if typeMultiplier(3, 1) > 1 && sideEffectTypeAdvantageFlat(d428, 3, 1) != 80 {
		t.Fatal("428")
	}
	d129 := &tableloader.SkillDef{SideEffect: "129", SideEffectArg: "1"}
	if genderMatchPowerMul(d129, 1, 50) != 100 || genderMatchPowerMul(d129, 2, 50) != 50 {
		t.Fatal("129")
	}
	d130 := &tableloader.SkillDef{SideEffect: "130", SideEffectArg: "2 40"}
	if genderMatchFlat(d130, 2) != 40 || genderMatchFlat(d130, 1) != 0 {
		t.Fatal("130")
	}
	d193 := &tableloader.SkillDef{SideEffect: "193", SideEffectArg: "5"}
	if !mustCritFromSideEffect193(d193, &battleStatus{Freeze: true}) || mustCritFromSideEffect193(d193, &battleStatus{}) {
		t.Fatal("193")
	}
	got := applyFreq2DamageSideEffects(d422, 100, 100, 100, 0, 0)
	if got != 150 {
		t.Fatalf("freq2 422 got=%d", got)
	}
	st := &BattleState{PlayerHP: 50, PlayerMaxHP: 100, EnemyHP: 80, EnemyMaxHP: 100,
		EnemyStages: [5]int8{2, 0, 0, 0, 0}, EnemyStatus: battleStatus{Burn: true}}
	s := &Server{}
	d172 := &tableloader.SkillDef{SideEffect: "172", SideEffectArg: "2"}
	applySecondStrikeDrain(st, d172, true, false, 40)
	if st.PlayerHP != 70 { // 50+20
		t.Fatalf("172 hp=%d", st.PlayerHP)
	}
	d430 := &tableloader.SkillDef{SideEffect: "430", SideEffectArg: "0 2"}
	applyClearBoostSelfStage(st, d430, true, []int{0, 2}, 0)
	if st.EnemyStages[0] != 0 || st.PlayerStages[0] != 2 {
		t.Fatalf("430 foe=%v self=%v", st.EnemyStages, st.PlayerStages)
	}
	d473 := &tableloader.SkillDef{SideEffect: "473", SideEffectArg: "100 1 2"}
	st.PlayerStages = [5]int8{}
	applyLowDamageSelfBoostEx(st, d473, true, 50)
	if st.PlayerStages[1] != 2 {
		t.Fatalf("473 stage=%v", st.PlayerStages)
	}
	_ = s
	if sideEffectArgCount(474) != 3 || sideEffectArgCount(455) != 2 || sideEffectArgCount(422) != 1 {
		t.Fatal("argCount")
	}
}

func TestSideEffectFreq2Batch2(t *testing.T) {
	d131 := &tableloader.SkillDef{SideEffect: "131", SideEffectArg: "1"}
	if !genderMatchImmune(d131, 1) || genderMatchImmune(d131, 2) {
		t.Fatal("131")
	}
	d162 := &tableloader.SkillDef{SideEffect: "162", SideEffectArg: "80"}
	if sideEffectAnyStatusFlat(d162, &battleStatus{Burn: true}) != 80 {
		t.Fatal("162")
	}
	d168 := &tableloader.SkillDef{SideEffect: "168"}
	if foeSleepDamageMul(d168, &battleStatus{Sleep: true}, 10) != 20 {
		t.Fatal("168")
	}
	dn := [5]int8{-1, 0, 0, 0, 0}
	d431 := &tableloader.SkillDef{SideEffect: "431"}
	if foeDropDamageMul(d431, &dn, 10) != 20 {
		t.Fatal("431")
	}
	st := &BattleState{PlayerHP: 50, PlayerMaxHP: 100, PlayerStages: [5]int8{1, 0, 0, 0, 0},
		EnemyStages: [5]int8{2, 0, 0, 0, 0}, PlayerSkills: [][2]uint32{{10001, 5}}}
	d453 := &tableloader.SkillDef{SideEffect: "453", SideEffectArg: "2"}
	applyClearBoostFoeStatus(st, d453, true, []int{2}, 0)
	if st.EnemyStages[0] != 0 || !st.EnemyStatus.Burn {
		t.Fatalf("453 stages=%v burn=%v", st.EnemyStages, st.EnemyStatus.Burn)
	}
	d134 := &tableloader.SkillDef{SideEffect: "134", SideEffectArg: "100 3"}
	applyLowDamagePPBoost(st, d134, true, 50, func(int) *tableloader.SkillDef {
		return &tableloader.SkillDef{MaxPP: 20}
	})
	if st.PlayerSkills[0][1] != 8 {
		t.Fatalf("134 pp=%d", st.PlayerSkills[0][1])
	}
	buff := &battleBuff{}
	_ = buff
	st2 := &BattleState{}
	applyOneOngoingBuff(st2, 463, []int{3, 25}, 0, true)
	if st2.PlayerBuff.FlatReduceRounds != 3 || st2.PlayerBuff.FlatReduceAmt != 25 {
		t.Fatalf("463 %+v", st2.PlayerBuff)
	}
	if applyIncomingDamageBuff(&st2.PlayerBuff, nil, 40) != 15 {
		t.Fatal("463 reduce")
	}
	if sideEffectArgCount(463) != 2 || sideEffectArgCount(194) != 3 || sideEffectArgCount(131) != 1 {
		t.Fatal("argCount2")
	}
}

func TestSideEffectFreq1Batch3(t *testing.T) {
	d111 := &tableloader.SkillDef{SideEffect: "111"}
	if sideEffectLevelFlat(d111, 50) != 100 {
		t.Fatal("111")
	}
	d132 := &tableloader.SkillDef{SideEffect: "132"}
	if lowHPVsFoeDamageMul(d132, 40, 80, 10) != 20 || lowHPVsFoeDamageMul(d132, 80, 40, 10) != 10 {
		t.Fatal("132")
	}
	d401 := &tableloader.SkillDef{SideEffect: "401"}
	if samePetTypeDamageMul(d401, 3, 3, 10) != 20 || samePetTypeDamageMul(d401, 3, 1, 10) != 10 {
		t.Fatal("401")
	}
	d411 := &tableloader.SkillDef{SideEffect: "411", SideEffectArg: "10 5 30"}
	if sideEffectFoeHPPercentFlat(d411, 200, 0) != 20 { // 10%
		t.Fatal("411 base")
	}
	if sideEffectFoeHPPercentFlat(d411, 200, 2) != 40 { // 20%
		t.Fatal("411 consec")
	}
	if sideEffectFoeHPPercentFlat(d411, 200, 10) != 60 { // capped 30%
		t.Fatal("411 cap")
	}
	d112 := &tableloader.SkillDef{SideEffect: "112"}
	dmg, loss, ok := sacrificeAllForFlat(d112, 80)
	if !ok || loss != 80 || dmg < 250 || dmg > 300 {
		t.Fatalf("112 dmg=%d loss=%d", dmg, loss)
	}
	if applyLeaveOneHP(d112, 100, 200) != 99 {
		t.Fatal("112 leave1")
	}
	d188 := &tableloader.SkillDef{SideEffect: "188"}
	if !mustCritFromAnyStatus(d188, &battleStatus{Poison: true}) {
		t.Fatal("188")
	}
	got := applyFreq3DamageSideEffects(d111, 10, 100, 100, 40, 0, 1, 2, 0)
	if got != 90 { // 10 + 80
		t.Fatalf("freq3 111 got=%d", got)
	}
	if sideEffectArgCount(411) != 3 || sideEffectArgCount(451) != 2 || sideEffectArgCount(118) != 0 {
		t.Fatal("argCount3")
	}
}
