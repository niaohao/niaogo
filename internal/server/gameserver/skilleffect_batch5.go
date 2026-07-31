package gameserver

import (
	"math/rand"

	"niaohao/server/internal/tableloader"
)

// --- SideEffect 无 AS / 低频批（参考服 effects + SkillXML 样例）---

// sideEffect484HitMul SideEffect 484：连击倍率（伤害 ×hits）。
func sideEffect484HitMul(d *tableloader.SkillDef) int {
	if d == nil || !skillHasSideEffect(d, 484) {
		return 1
	}
	args := sideEffectArgsFor(d, 484)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	base, inc, maxHits := 2, 1, 5
	if len(args) >= 1 && args[0] > 0 {
		base = args[0]
	}
	if len(args) >= 2 && args[1] > 0 {
		inc = args[1]
	}
	if len(args) >= 3 && args[2] > 0 {
		maxHits = args[2]
	}
	hits := base
	if inc > 0 && base > 1 {
		hits = base + inc*(base-1)
	}
	if hits > maxHits {
		hits = maxHits
	}
	if hits < 1 {
		hits = 1
	}
	return hits
}

// sideEffect488LowHPMul SideEffect 488：对手体力低于阈值则伤害 +n%。
func sideEffect488LowHPMul(d *tableloader.SkillDef, foeHP, dmg uint32) uint32 {
	if d == nil || dmg == 0 || !skillHasSideEffect(d, 488) {
		return dmg
	}
	args := sideEffectArgsFor(d, 488)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	thr, pct := 400, 10
	if len(args) >= 1 && args[0] > 0 {
		thr = args[0]
	}
	if len(args) >= 2 && args[1] > 0 {
		pct = args[1]
	}
	if foeHP >= uint32(thr) {
		return dmg
	}
	return dmg * uint32(100+pct) / 100
}

// applyEffect795DamageBoost SideEffect 795：同技叠伤（每用 +m%，上限 n%）。
func applyEffect795DamageBoost(d *tableloader.SkillDef, uses *byte, dmg uint32) uint32 {
	if d == nil || dmg == 0 || uses == nil || !skillHasSideEffect(d, 795) {
		return dmg
	}
	args := sideEffectArgsFor(d, 795)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	perUse, cap := 20, 100
	if len(args) >= 1 && args[0] > 0 {
		perUse = args[0]
	}
	if len(args) >= 2 && args[1] > 0 {
		cap = args[1]
	}
	if *uses < 255 {
		*uses++
	}
	bonus := int(*uses) * perUse
	if bonus > cap {
		bonus = cap
	}
	if bonus <= 0 {
		return dmg
	}
	return dmg * uint32(100+bonus) / 100
}

// dvPower1470 SideEffect 1470：按个体映射威力。
func dvPower1470(d *tableloader.SkillDef, base, dv int) int {
	if d == nil || !skillHasSideEffect(d, 1470) {
		return base
	}
	switch {
	case dv <= 10:
		return 100
	case dv <= 20:
		return 120
	case dv <= 25:
		return 140
	case dv <= 30:
		return 160
	default:
		return 180
	}
}

// powerBonus2237 SideEffect 2237：对方每多存活 1 只，威力 +n。
func powerBonus2237(d *tableloader.SkillDef, foeAliveExtra int) int {
	if d == nil || foeAliveExtra <= 0 || !skillHasSideEffect(d, 2237) {
		return 0
	}
	args := sideEffectArgsFor(d, 2237)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	step := 100
	if len(args) >= 1 && args[0] > 0 {
		step = args[0]
	}
	return step * foeAliveExtra
}

func applyFreq5DamageSideEffects(d *tableloader.SkillDef, dmg, foeHP uint32, uses795 *byte) uint32 {
	if d == nil || dmg == 0 {
		return dmg
	}
	if m := sideEffect484HitMul(d); m > 1 {
		dmg *= uint32(m)
	}
	dmg = sideEffect488LowHPMul(d, foeHP, dmg)
	dmg = applyEffect795DamageBoost(d, uses795, dmg)
	return dmg
}

func applyClearBoostFullHeal485(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool) {
	if st == nil || d == nil || !skillHasSideEffect(d, 485) {
		return
	}
	foe := pickFoeStages(st, playerIsAtk)
	cleared := false
	for i := range foe {
		if foe[i] > 0 {
			foe[i] = 0
			cleared = true
		}
	}
	if !cleared {
		return
	}
	if playerIsAtk {
		st.PlayerHP = st.PlayerMaxHP
	} else {
		st.EnemyHP = st.EnemyMaxHP
	}
}

func applyHighHPAtkBoost487(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	thr, delta := 400, 1
	if argOff < len(args) && args[argOff] > 0 {
		thr = args[argOff]
		argOff++
	}
	if argOff < len(args) && args[argOff] > 0 {
		delta = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 487) {
		return argOff
	}
	foeHP := st.EnemyHP
	if !playerIsAtk {
		foeHP = st.PlayerHP
	}
	if foeHP <= uint32(thr) {
		return argOff
	}
	stages := pickSelfStages(st, playerIsAtk)
	stages[stageAtk] = int8(clampStage(int(stages[stageAtk]) + delta))
	return argOff
}

func applyClearFoeBoost494(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool) {
	if st == nil || d == nil || !skillHasSideEffect(d, 494) {
		return
	}
	if stageDropImmuneFromBuff(pickFoeBuff(st, playerIsAtk)) {
		return
	}
	clearPositiveStages(pickFoeStages(st, playerIsAtk))
}

func applyStatusExecute495(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	idx, chance := 2, 30
	if argOff < len(args) {
		idx = args[argOff]
		argOff++
	}
	if argOff < len(args) && args[argOff] > 0 {
		chance = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 495) {
		return argOff
	}
	foe := pickFoeStatus(st, playerIsAtk)
	if !statusByTableIndex(foe, idx) {
		return argOff
	}
	if chance < 100 && rand.Intn(100) >= chance {
		return argOff
	}
	petID := st.EnemyID
	if !playerIsAtk {
		petID = st.PlayerPetID
	}
	if !canApplyEnemyStatus(petID, 36) {
		return argOff
	}
	if playerIsAtk {
		st.EnemyHP = 0
	} else {
		st.PlayerHP = 0
	}
	return argOff
}

func applyChanceOHKO691(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	chance := 5
	if argOff < len(args) && args[argOff] > 0 {
		chance = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 691) {
		return argOff
	}
	if chance < 100 && rand.Intn(100) >= chance {
		return argOff
	}
	petID := st.EnemyID
	if !playerIsAtk {
		petID = st.PlayerPetID
	}
	if !canApplyEnemyStatus(petID, 36) {
		return argOff
	}
	if playerIsAtk {
		st.EnemyHP = 0
	} else {
		st.PlayerHP = 0
	}
	return argOff
}

func applyFirstStrikePPDrain700(st *BattleState, d *tableloader.SkillDef, playerIsAtk, wentFirst bool, args []int, argOff int) int {
	cut := 2
	if argOff < len(args) && args[argOff] > 0 {
		cut = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !wentFirst || !skillHasSideEffect(d, 700) {
		return argOff
	}
	skills := st.EnemySkills
	if !playerIsAtk {
		skills = st.PlayerSkills
	}
	for i := range skills {
		if skills[i][0] == 0 {
			continue
		}
		if skills[i][1] > uint32(cut) {
			skills[i][1] -= uint32(cut)
		} else {
			skills[i][1] = 0
		}
	}
	if playerIsAtk {
		st.EnemySkills = skills
	} else {
		st.PlayerSkills = skills
	}
	return argOff
}

func applyLowHPSwap773(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool) {
	if st == nil || d == nil || !skillHasSideEffect(d, 773) {
		return
	}
	selfHP, foeHP := &st.PlayerHP, &st.EnemyHP
	selfMax, foeMax := st.PlayerMaxHP, st.EnemyMaxHP
	if !playerIsAtk {
		selfHP, foeHP = &st.EnemyHP, &st.PlayerHP
		selfMax, foeMax = st.EnemyMaxHP, st.PlayerMaxHP
	}
	if *selfHP >= *foeHP {
		return
	}
	p, e := *selfHP, *foeHP
	*selfHP, *foeHP = e, p
	if *selfHP > selfMax {
		*selfHP = selfMax
	}
	if *foeHP > foeMax {
		*foeHP = foeMax
	}
}

func applyHighHPStatus935(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	code := 0
	if argOff < len(args) {
		code = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 935) {
		return argOff
	}
	selfHP, foeHP := st.PlayerHP, st.EnemyHP
	if !playerIsAtk {
		selfHP, foeHP = st.EnemyHP, st.PlayerHP
	}
	if selfHP <= foeHP {
		return argOff
	}
	foe := pickFoeStatus(st, playerIsAtk)
	if statusImmuneFromBuff(pickFoeBuff(st, playerIsAtk)) {
		return argOff
	}
	setStatusByTableIndex(foe, code)
	return argOff
}

func applyDispelWithAttrBlock976(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	n := 1
	if argOff < len(args) && args[argOff] > 0 {
		n = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 976) {
		return argOff
	}
	foeBuff := pickFoeBuff(st, playerIsAtk)
	had := foeBuff != nil && *foeBuff != (battleBuff{})
	clearFoeOngoingBuffs(st, playerIsAtk)
	if had {
		foeBuff = pickFoeBuff(st, playerIsAtk)
		foeBuff.BlockAttrSkillRounds = clampRounds(n)
	}
	return argOff
}

func applySecondStrikeDispel1083(st *BattleState, d *tableloader.SkillDef, playerIsAtk, wentFirst bool) {
	if st == nil || d == nil || wentFirst || !skillHasSideEffect(d, 1083) {
		return
	}
	clearFoeOngoingBuffs(st, playerIsAtk)
}

func clearAllStages(stages *[5]int8) {
	if stages == nil {
		return
	}
	for i := range stages {
		stages[i] = 0
	}
}

func applyClearAllStagesAbsorb1211(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	absorb := 300
	if argOff < len(args) && args[argOff] > 0 {
		absorb = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 1211) {
		return argOff
	}
	selfSt, foeSt := pickSelfStages(st, playerIsAtk), pickFoeStages(st, playerIsAtk)
	changed := false
	for i := range selfSt {
		if selfSt[i] != 0 {
			changed = true
		}
	}
	for i := range foeSt {
		if foeSt[i] != 0 {
			changed = true
		}
	}
	clearAllStages(selfSt)
	clearAllStages(foeSt)
	if changed {
		self := pickSelfBuff(st, playerIsAtk)
		self.DamageAbsorb += uint32(absorb)
	}
	return argOff
}

func applyAbnormalExtraStatus1248(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	chance, idx := 100, 0
	if argOff < len(args) && args[argOff] > 0 {
		chance = args[argOff]
		argOff++
	}
	if argOff < len(args) {
		idx = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 1248) {
		return argOff
	}
	foe := pickFoeStatus(st, playerIsAtk)
	if !hasAnyAbnormalStatus(foe) {
		return argOff
	}
	if chance < 100 && rand.Intn(100) >= chance {
		return argOff
	}
	if statusImmuneFromBuff(pickFoeBuff(st, playerIsAtk)) {
		return argOff
	}
	setStatusByTableIndex(foe, idx)
	return argOff
}

func applyNoStatusDrain1257(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	div := 3
	if argOff < len(args) && args[argOff] > 0 {
		div = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 1257) {
		return argOff
	}
	if div < 1 {
		div = 1
	}
	foeStatus := pickFoeStatus(st, playerIsAtk)
	if hasAnyAbnormalStatus(foeStatus) {
		return argOff
	}
	foeHP, foeMax := &st.EnemyHP, st.EnemyMaxHP
	selfHP, selfMax := &st.PlayerHP, &st.PlayerMaxHP
	if !playerIsAtk {
		foeHP, foeMax = &st.PlayerHP, st.PlayerMaxHP
		selfHP, selfMax = &st.EnemyHP, &st.EnemyMaxHP
	}
	drain := foeMax / uint32(div)
	if drain < 1 {
		drain = 1
	}
	actual := applyDamage(foeHP, drain)
	applyHealCap(selfHP, selfMax, actual)
	return argOff
}

func applyChancePPDrain1603(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	chance, cut := 100, 1
	if argOff < len(args) && args[argOff] > 0 {
		chance = args[argOff]
		argOff++
	}
	if argOff < len(args) && args[argOff] > 0 {
		cut = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 1603) {
		return argOff
	}
	if chance < 100 && rand.Intn(100) >= chance {
		return argOff
	}
	skills := st.EnemySkills
	if !playerIsAtk {
		skills = st.PlayerSkills
	}
	for i := range skills {
		if skills[i][0] == 0 {
			continue
		}
		if skills[i][1] > uint32(cut) {
			skills[i][1] -= uint32(cut)
		} else {
			skills[i][1] = 0
		}
	}
	if playerIsAtk {
		st.EnemySkills = skills
	} else {
		st.PlayerSkills = skills
	}
	return argOff
}

func applyChanceStatus1605(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	chance, idx := 100, 0
	if argOff < len(args) && args[argOff] > 0 {
		chance = args[argOff]
		argOff++
	}
	if argOff < len(args) {
		idx = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 1605) {
		return argOff
	}
	if chance < 100 && rand.Intn(100) >= chance {
		return argOff
	}
	if statusImmuneFromBuff(pickFoeBuff(st, playerIsAtk)) {
		return argOff
	}
	// 未知状态码（如臣服 31）近似为疲惫
	if idx > 8 {
		idx = 7
	}
	setStatusByTableIndex(pickFoeStatus(st, playerIsAtk), idx)
	return argOff
}

func clearSelfOngoingBuffs(st *BattleState, playerIsAtk bool) {
	if st == nil {
		return
	}
	*pickSelfBuff(st, playerIsAtk) = battleBuff{}
	if playerIsAtk {
		st.PlayerChargeSkill = 0
		st.PlayerChargeReady = false
		st.PlayerDoomRounds = 0
		st.PlayerSkillFail = false
		st.PlayerCritBonusRounds = 0
		st.PlayerTypeOverrideRounds = 0
	} else {
		st.EnemyChargeSkill = 0
		st.EnemyChargeReady = false
		st.EnemyDoomRounds = 0
		st.EnemySkillFail = false
		st.EnemyCritBonusRounds = 0
		st.EnemyTypeOverrideRounds = 0
	}
}

func applyDispelBothBoost1850(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	hits, pct := 1, 50
	if argOff < len(args) && args[argOff] > 0 {
		hits = args[argOff]
		argOff++
	}
	if argOff < len(args) && args[argOff] > 0 {
		pct = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 1850) {
		return argOff
	}
	selfHad := *pickSelfBuff(st, playerIsAtk) != (battleBuff{})
	foeHad := *pickFoeBuff(st, playerIsAtk) != (battleBuff{})
	clearSelfOngoingBuffs(st, playerIsAtk)
	clearFoeOngoingBuffs(st, playerIsAtk)
	if selfHad || foeHad {
		self := pickSelfBuff(st, playerIsAtk)
		self.OutDmgBoostHits = clampRounds(hits)
		self.OutDmgBoostPct = byte(pct)
	}
	return argOff
}

func applyMaxHPDrain1925(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	div := 3
	if argOff < len(args) && args[argOff] > 0 {
		div = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 1925) {
		return argOff
	}
	if div < 1 {
		div = 1
	}
	foeHP, foeMax := &st.EnemyHP, st.EnemyMaxHP
	selfHP, selfMax := &st.PlayerHP, &st.PlayerMaxHP
	if !playerIsAtk {
		foeHP, foeMax = &st.PlayerHP, st.PlayerMaxHP
		selfHP, selfMax = &st.EnemyHP, &st.EnemyMaxHP
	}
	drain := foeMax / uint32(div)
	if drain < 1 {
		drain = 1
	}
	actual := applyDamage(foeHP, drain)
	applyHealCap(selfHP, selfMax, actual)
	return argOff
}

func applySupportDouble2236(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	n := 1
	if argOff < len(args) && args[argOff] > 0 {
		n = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 2236) {
		return argOff
	}
	self := pickSelfBuff(st, playerIsAtk)
	self.SupportDoubleHits = clampRounds(n)
	return argOff
}
