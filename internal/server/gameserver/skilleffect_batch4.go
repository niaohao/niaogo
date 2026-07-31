package gameserver

import (
	"math/rand"

	"niaohao/server/internal/tableloader"
)

// —— 续补批（2026-07-30 #4：剩余带 AS 全接）——

func applyWeaknessLayer(stages *[5]int8) {
	if stages == nil {
		return
	}
	stat := rand.Intn(stageSpd + 1)
	stages[stat] = int8(clampStage(int(stages[stat]) - 1))
}

func applyChanceWeakness(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	chance := 30
	if argOff < len(args) {
		chance = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 103) {
		return argOff
	}
	if chance > 100 {
		chance = 100
	}
	if rand.Intn(100) >= chance {
		return argOff
	}
	if stageDropImmuneFromBuff(pickFoeBuff(st, playerIsAtk)) {
		return argOff
	}
	applyWeaknessLayer(pickFoeStages(st, playerIsAtk))
	return argOff
}

func setFlammable(st *battleStatus) {
	if st != nil {
		st.Flammable = true
	}
}

func applyChanceFlammable(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	chance := 30
	if argOff < len(args) {
		chance = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 114) {
		return argOff
	}
	if chance > 100 {
		chance = 100
	}
	if rand.Intn(100) >= chance {
		return argOff
	}
	if statusImmuneFromBuff(pickFoeBuff(st, playerIsAtk)) {
		return argOff
	}
	setFlammable(pickFoeStatus(st, playerIsAtk))
	return argOff
}

func flammableFireMul(d *tableloader.SkillDef, foe *battleStatus, dmg uint32) uint32 {
	if d == nil || foe == nil || dmg == 0 || !foe.Flammable || d.Type != 3 {
		return dmg
	}
	return dmg * 3 / 2
}

func dvSkillPower(d *tableloader.SkillDef, base, dv int) int {
	if d == nil {
		return base
	}
	if skillHasSideEffect(d, 113) {
		if dv < 0 {
			dv = 0
		}
		p := dv * 5
		if p < 1 {
			p = 1
		}
		return p
	}
	if skillHasSideEffect(d, 1901) {
		if dv < 0 {
			dv = 0
		}
		p := dv * 5
		if p > 155 {
			p = 155
		}
		if p < 1 {
			p = 1
		}
		return p
	}
	return base
}

func sideEffectSelfHPPercentFlat(d *tableloader.SkillDef, selfHP uint32) uint32 {
	if d == nil || selfHP == 0 || !skillHasSideEffect(d, 192) {
		return 0
	}
	args := sideEffectArgsFor(d, 192)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	pct := 10
	if len(args) >= 1 && args[0] > 0 {
		pct = args[0]
	}
	return selfHP * uint32(pct) / 100
}

func sideEffectStackFlat429(d *tableloader.SkillDef, consec uint32) uint32 {
	if d == nil || !skillHasSideEffect(d, 429) {
		return 0
	}
	args := sideEffectArgsFor(d, 429)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	base, step, maxFlat := 0, 0, 0
	if len(args) >= 1 {
		base = args[0]
	}
	if len(args) >= 2 {
		step = args[1]
	}
	if len(args) >= 3 {
		maxFlat = args[2]
	}
	flat := base + int(consec)*step
	if maxFlat > 0 && flat > maxFlat {
		flat = maxFlat
	}
	if flat < 1 {
		return 0
	}
	return uint32(flat)
}

func sideEffectMinDamage447(d *tableloader.SkillDef, dmg uint32) uint32 {
	if d == nil || !skillHasSideEffect(d, 447) {
		return dmg
	}
	args := sideEffectArgsFor(d, 447)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	min := 0
	if len(args) >= 1 {
		min = args[0]
	}
	if min > 0 && int(dmg) < min {
		return uint32(min)
	}
	return dmg
}

func sideEffectDefPercentFlat(d *tableloader.SkillDef, foeDef int) uint32 {
	if d == nil || foeDef <= 0 || !skillHasSideEffect(d, 459) {
		return 0
	}
	args := sideEffectArgsFor(d, 459)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	pct := 50
	if len(args) >= 1 && args[0] > 0 {
		pct = args[0]
	}
	return uint32(foeDef*pct) / 100
}

func powerDoubleIfSelfDrop(d *tableloader.SkillDef, selfStages *[5]int8, dmg uint32) (uint32, bool) {
	if d == nil || dmg == 0 || !skillHasSideEffect(d, 468) || !hasNegativeStages(selfStages) {
		return dmg, false
	}
	clearNegativeStages(selfStages)
	return dmg * 2, true
}

func applyFreq4DamageSideEffects(d *tableloader.SkillDef, dmg, selfHP uint32, selfStages *[5]int8, foeDef int, consec uint32, foe *battleStatus) uint32 {
	if d == nil {
		return dmg
	}
	if dmg > 0 {
		dmg = flammableFireMul(d, foe, dmg)
		if nd, ok := powerDoubleIfSelfDrop(d, selfStages, dmg); ok {
			dmg = nd
		}
	}
	dmg += sideEffectSelfHPPercentFlat(d, selfHP)
	dmg += sideEffectStackFlat429(d, consec)
	dmg += sideEffectDefPercentFlat(d, foeDef)
	dmg = sideEffectMinDamage447(d, dmg)
	return dmg
}

func applyCoinFlipHPCut(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	denom := 4
	if argOff < len(args) && args[argOff] > 0 {
		denom = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 120) {
		return argOff
	}
	if denom < 1 {
		denom = 1
	}
	if rand.Intn(2) == 0 {
		// 对方
		if playerIsAtk {
			cut := st.EnemyMaxHP / uint32(denom)
			if cut < 1 {
				cut = 1
			}
			_ = applyDamage(&st.EnemyHP, cut)
		} else {
			cut := st.PlayerMaxHP / uint32(denom)
			if cut < 1 {
				cut = 1
			}
			_ = applyDamage(&st.PlayerHP, cut)
		}
	} else {
		if playerIsAtk {
			cut := st.PlayerMaxHP / uint32(denom)
			if cut < 1 {
				cut = 1
			}
			_ = applyDamage(&st.PlayerHP, cut)
		} else {
			cut := st.EnemyMaxHP / uint32(denom)
			if cut < 1 {
				cut = 1
			}
			_ = applyDamage(&st.EnemyHP, cut)
		}
	}
	return argOff
}

func invertPositiveToNegative(stages *[5]int8) {
	if stages == nil {
		return
	}
	for i, v := range stages {
		if v > 0 {
			stages[i] = int8(-v)
		}
	}
}

func applyInvertFoeBoosts(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool) {
	if st == nil || d == nil || !skillHasSideEffect(d, 143) {
		return
	}
	invertPositiveToNegative(pickFoeStages(st, playerIsAtk))
}

func applyDualStageChange196(st *BattleState, d *tableloader.SkillDef, playerIsAtk, wentFirst bool, args []int, argOff int) int {
	// args: stat1 chance1 delta1 stat2 chance2 delta2
	need := 6
	if argOff+need > len(args) {
		need = len(args) - argOff
	}
	if need < 0 {
		need = 0
	}
	slice := args[argOff : argOff+need]
	argOff += need
	if st == nil || d == nil || !skillHasSideEffect(d, 196) {
		return argOff
	}
	applyOne := func(stat, chance, delta int) {
		if chance > 100 {
			chance = 100
		}
		if stat < 0 || stat > stageSpd || rand.Intn(100) >= chance {
			return
		}
		if stageDropImmuneFromBuff(pickFoeBuff(st, playerIsAtk)) && delta < 0 {
			return
		}
		stages := pickFoeStages(st, playerIsAtk)
		stages[stat] = int8(clampStage(int(stages[stat]) + delta))
	}
	stat, chance, delta := 0, 50, -1
	if len(slice) >= 1 {
		stat = slice[0]
	}
	if len(slice) >= 2 {
		chance = slice[1]
	}
	if len(slice) >= 3 {
		delta = slice[2]
	}
	applyOne(stat, chance, delta)
	if wentFirst && len(slice) >= 6 {
		applyOne(slice[3], slice[4], slice[5])
	}
	return argOff
}

func applyPartyOrSelfHeal201(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	// AS args=2，描述用 [1] 作分母
	denom := 4
	if argOff < len(args) {
		argOff++ // skip [0]
	}
	if argOff < len(args) && args[argOff] > 0 {
		denom = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 201) {
		return argOff
	}
	if denom < 1 {
		denom = 1
	}
	if playerIsAtk {
		heal := st.PlayerMaxHP / uint32(denom)
		if heal < 1 {
			heal = 1
		}
		applyHealCap(&st.PlayerHP, &st.PlayerMaxHP, heal)
	} else {
		heal := st.EnemyMaxHP / uint32(denom)
		if heal < 1 {
			heal = 1
		}
		applyHealCap(&st.EnemyHP, &st.EnemyMaxHP, heal)
	}
	return argOff
}

func applyTransferBoostsOnKO421(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, foeHPBefore uint32) {
	if st == nil || d == nil || foeHPBefore == 0 || !skillHasSideEffect(d, 421) {
		return
	}
	foeDead := false
	if playerIsAtk {
		foeDead = st.EnemyHP == 0
	} else {
		foeDead = st.PlayerHP == 0
	}
	if !foeDead {
		return
	}
	self, foe := pickSelfStages(st, playerIsAtk), pickFoeStages(st, playerIsAtk)
	for i := range foe {
		if foe[i] > 0 {
			self[i] = int8(clampStage(int(self[i]) + int(foe[i])))
			foe[i] = 0
		}
	}
}

func applyPPSwap444(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, skillOf func(int) *tableloader.SkillDef) {
	if st == nil || d == nil || !skillHasSideEffect(d, 444) {
		return
	}
	selfSkills, foeSkills := st.PlayerSkills, st.EnemySkills
	if !playerIsAtk {
		selfSkills, foeSkills = st.EnemySkills, st.PlayerSkills
	}
	for i := range foeSkills {
		if foeSkills[i][0] != 0 && foeSkills[i][1] > 0 {
			foeSkills[i][1]--
		}
	}
	for i := range selfSkills {
		if selfSkills[i][0] == 0 {
			continue
		}
		selfSkills[i][1]++
		if skillOf != nil {
			if def := skillOf(int(selfSkills[i][0])); def != nil && def.MaxPP > 0 && selfSkills[i][1] > uint32(def.MaxPP) {
				selfSkills[i][1] = uint32(def.MaxPP)
			}
		}
	}
}

func applyDropCondStatus449(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	chance, idx := 50, 0
	if argOff < len(args) {
		chance = args[argOff]
		argOff++
	}
	if argOff < len(args) {
		idx = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 449) {
		return argOff
	}
	if !hasNegativeStages(pickFoeStages(st, playerIsAtk)) {
		return argOff
	}
	if chance > 100 {
		chance = 100
	}
	if rand.Intn(100) >= chance {
		return argOff
	}
	foe := pickFoeStatus(st, playerIsAtk)
	if statusImmuneFromBuff(pickFoeBuff(st, playerIsAtk)) {
		return argOff
	}
	setStatusByTableIndex(foe, idx)
	return argOff
}

func applyRandomHealRange450(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	lo, hi := 50, 100
	if argOff < len(args) {
		lo = args[argOff]
		argOff++
	}
	if argOff < len(args) {
		hi = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 450) {
		return argOff
	}
	if hi < lo {
		hi = lo
	}
	heal := lo
	if hi > lo {
		heal = lo + rand.Intn(hi-lo+1)
	}
	if heal < 1 {
		return argOff
	}
	if playerIsAtk {
		applyHealCap(&st.PlayerHP, &st.PlayerMaxHP, uint32(heal))
	} else {
		applyHealCap(&st.EnemyHP, &st.EnemyMaxHP, uint32(heal))
	}
	return argOff
}

func applyFirstStrikeDrain458(st *BattleState, d *tableloader.SkillDef, playerIsAtk, wentFirst bool, lost uint32) {
	if st == nil || d == nil || !wentFirst || lost == 0 || !skillHasSideEffect(d, 458) {
		return
	}
	heal := lost / 2
	if heal < 1 {
		heal = 1
	}
	if playerIsAtk {
		applyHealCap(&st.PlayerHP, &st.PlayerMaxHP, heal)
	} else {
		applyHealCap(&st.EnemyHP, &st.EnemyMaxHP, heal)
	}
}

func applyFearWithBoostBonus460(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	chance, extra := 30, 20
	if argOff < len(args) {
		chance = args[argOff]
		argOff++
	}
	if argOff < len(args) {
		extra = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 460) {
		return argOff
	}
	if hasPositiveStages(pickFoeStages(st, playerIsAtk)) {
		chance += extra
	}
	if chance > 100 {
		chance = 100
	}
	if rand.Intn(100) >= chance {
		return argOff
	}
	foe := pickFoeStatus(st, playerIsAtk)
	if statusImmuneFromBuff(pickFoeBuff(st, playerIsAtk)) {
		return argOff
	}
	setStatusByTableIndex(foe, 6) // 害怕
	return argOff
}

func applyRisingTired465(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, consec int, args []int, argOff int) int {
	chance, rounds, step, maxC := 20, 1, 5, 50
	need := 4
	if argOff+need > len(args) {
		need = len(args) - argOff
	}
	if need < 0 {
		need = 0
	}
	slice := args[argOff : argOff+need]
	argOff += need
	if len(slice) >= 1 {
		chance = slice[0]
	}
	if len(slice) >= 2 && slice[1] > 0 {
		rounds = slice[1]
	}
	if len(slice) >= 3 {
		step = slice[2]
	}
	if len(slice) >= 4 {
		maxC = slice[3]
	}
	_ = rounds
	if st == nil || d == nil || !skillHasSideEffect(d, 465) {
		return argOff
	}
	if consec > 1 {
		chance += (consec - 1) * step
	}
	if maxC > 0 && chance > maxC {
		chance = maxC
	}
	if chance > 100 {
		chance = 100
	}
	if rand.Intn(100) >= chance {
		return argOff
	}
	foe := pickFoeStatus(st, playerIsAtk)
	if statusImmuneFromBuff(pickFoeBuff(st, playerIsAtk)) {
		return argOff
	}
	setStatusByTableIndex(foe, 7)
	return argOff
}

func applyFlatHeal466(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	amt := 50
	if argOff < len(args) {
		amt = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 466) || amt <= 0 {
		return argOff
	}
	if playerIsAtk {
		applyHealCap(&st.PlayerHP, &st.PlayerMaxHP, uint32(amt))
	} else {
		applyHealCap(&st.EnemyHP, &st.EnemyMaxHP, uint32(amt))
	}
	return argOff
}

func applySecondStrikeFlatHeal476(st *BattleState, d *tableloader.SkillDef, playerIsAtk, wentFirst bool, args []int, argOff int) int {
	amt := 50
	if argOff < len(args) {
		amt = args[argOff]
		argOff++
	}
	if st == nil || d == nil || wentFirst || !skillHasSideEffect(d, 476) || amt <= 0 {
		return argOff
	}
	if playerIsAtk {
		applyHealCap(&st.PlayerHP, &st.PlayerMaxHP, uint32(amt))
	} else {
		applyHealCap(&st.EnemyHP, &st.EnemyMaxHP, uint32(amt))
	}
	return argOff
}

func applyChancePriority482(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	chance, rounds := 50, 1
	if argOff < len(args) {
		chance = args[argOff]
		argOff++
	}
	if argOff < len(args) && args[argOff] > 0 {
		rounds = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 482) {
		return argOff
	}
	if chance > 100 {
		chance = 100
	}
	if rand.Intn(100) >= chance {
		return argOff
	}
	pickSelfBuff(st, playerIsAtk).PriorityRounds = clampRounds(rounds)
	return argOff
}

func applyFlatReduceNext508(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	flat := 50
	if argOff < len(args) && args[argOff] > 0 {
		flat = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 508) {
		return argOff
	}
	pickSelfBuff(st, playerIsAtk).FlatReduceNext = uint32(flat)
	return argOff
}

func applyStatusDrain687(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, lost uint32, args []int, argOff int) int {
	idx, pct := 0, 50
	if argOff < len(args) {
		idx = args[argOff]
		argOff++
	}
	if argOff < len(args) && args[argOff] > 0 {
		pct = args[argOff]
		argOff++
	}
	if st == nil || d == nil || lost == 0 || !skillHasSideEffect(d, 687) {
		return argOff
	}
	if !statusByTableIndexEx(pickFoeStatus(st, playerIsAtk), idx) {
		return argOff
	}
	heal := lost * uint32(pct) / 100
	if heal < 1 {
		heal = 1
	}
	if playerIsAtk {
		applyHealCap(&st.PlayerHP, &st.PlayerMaxHP, heal)
	} else {
		applyHealCap(&st.EnemyHP, &st.EnemyMaxHP, heal)
	}
	return argOff
}

func applyHealAndDelayFull1635(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	heal, delay := 100, 3
	if argOff < len(args) {
		heal = args[argOff]
		argOff++
	}
	if argOff < len(args) && args[argOff] > 0 {
		delay = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 1635) {
		return argOff
	}
	if playerIsAtk {
		if heal > 0 {
			applyHealCap(&st.PlayerHP, &st.PlayerMaxHP, uint32(heal))
		}
		st.PlayerDelayedFullHealRounds = clampRounds(delay)
	} else {
		if heal > 0 {
			applyHealCap(&st.EnemyHP, &st.EnemyMaxHP, uint32(heal))
		}
		st.EnemyDelayedFullHealRounds = clampRounds(delay)
	}
	return argOff
}

func applyLowDamageMustCrit475(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, lost uint32, args []int, argOff int) int {
	thr, rounds := 100, 1
	if argOff < len(args) && args[argOff] > 0 {
		thr = args[argOff]
		argOff++
	}
	if argOff < len(args) && args[argOff] > 0 {
		rounds = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 475) {
		return argOff
	}
	if lost == 0 || int(lost) >= thr {
		return argOff
	}
	pickSelfBuff(st, playerIsAtk).MustCritRounds = clampRounds(rounds)
	return argOff
}

func applyFirstStrikeStatusImmune471(st *BattleState, d *tableloader.SkillDef, playerIsAtk, wentFirst bool, args []int, argOff int) int {
	rounds := 1
	if argOff < len(args) && args[argOff] > 0 {
		rounds = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !wentFirst || !skillHasSideEffect(d, 471) {
		return argOff
	}
	pickSelfBuff(st, playerIsAtk).ImmuneStatusRounds = clampRounds(rounds)
	return argOff
}

func applyLowHPMustCrit461(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	denom := 2
	if argOff < len(args) && args[argOff] > 0 {
		denom = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 461) {
		return argOff
	}
	hp, maxHP := st.EnemyHP, st.EnemyMaxHP
	if playerIsAtk {
		hp, maxHP = st.PlayerHP, st.PlayerMaxHP
	}
	if denom < 1 || maxHP == 0 || hp*uint32(denom) >= maxHP {
		return argOff
	}
	pickSelfBuff(st, playerIsAtk).MustCritRounds = clampRounds(3)
	return argOff
}

func applyLowHPPriority454(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	denom, pri := 2, 1
	if argOff < len(args) && args[argOff] > 0 {
		denom = args[argOff]
		argOff++
	}
	if argOff < len(args) && args[argOff] > 0 {
		pri = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 454) {
		return argOff
	}
	hp, maxHP := st.EnemyHP, st.EnemyMaxHP
	if playerIsAtk {
		hp, maxHP = st.PlayerHP, st.PlayerMaxHP
	}
	if denom < 1 || maxHP == 0 || hp*uint32(denom) >= maxHP {
		return argOff
	}
	_ = pri
	pickSelfBuff(st, playerIsAtk).PriorityRounds = clampRounds(1)
	return argOff
}

func applyNextEnterAtkDef202(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool) {
	if st == nil || d == nil || !skillHasSideEffect(d, 202) {
		return
	}
	if playerIsAtk {
		st.PlayerNextEnterAtkDefBoost = true
	}
}

func applyRewardCoins445(st *BattleState, d *tableloader.SkillDef) {
	if st == nil || d == nil || !skillHasSideEffect(d, 445) {
		return
	}
	st.RewardCoins445 = 500
}

func tickDelayedFullHeal(st *BattleState) {
	if st == nil {
		return
	}
	if st.PlayerDelayedFullHealRounds > 0 {
		st.PlayerDelayedFullHealRounds--
		if st.PlayerDelayedFullHealRounds == 0 {
			st.PlayerHP = st.PlayerMaxHP
		}
	}
	if st.EnemyDelayedFullHealRounds > 0 {
		st.EnemyDelayedFullHealRounds--
		if st.EnemyDelayedFullHealRounds == 0 {
			st.EnemyHP = st.EnemyMaxHP
		}
	}
}

func tickGrowAtkSpd(st *BattleState) {
	if st == nil {
		return
	}
	grow := func(b *battleBuff, stages *[5]int8) {
		if b == nil || stages == nil || b.GrowAtkSpdRounds == 0 {
			return
		}
		d := int(b.GrowAtkSpdDelta)
		if d == 0 {
			d = 1
		}
		stages[stageAtk] = int8(clampStage(int(stages[stageAtk]) + d))
		stages[stageSpd] = int8(clampStage(int(stages[stageSpd]) + d))
	}
	grow(&st.PlayerBuff, &st.PlayerStages)
	grow(&st.EnemyBuff, &st.EnemyStages)
}

func tickFoeStageDot(st *BattleState) {
	if st == nil {
		return
	}
	apply := func(atk *battleBuff, foeStages *[5]int8) {
		if atk == nil || foeStages == nil || atk.FoeStageDotRounds == 0 {
			return
		}
		for i := 0; i < 5; i++ {
			if atk.FoeStageDotDelta[i] != 0 {
				foeStages[i] = int8(clampStage(int(foeStages[i]) + int(atk.FoeStageDotDelta[i])))
			}
		}
	}
	apply(&st.PlayerBuff, &st.EnemyStages)
	apply(&st.EnemyBuff, &st.PlayerStages)
}

func tickSelfStageGrow(st *BattleState) {
	if st == nil {
		return
	}
	apply := func(b *battleBuff, stages *[5]int8) {
		if b == nil || stages == nil || b.SelfStageGrowRounds == 0 {
			return
		}
		for i := 0; i < 5; i++ {
			if b.SelfStageGrowDelta[i] != 0 {
				stages[i] = int8(clampStage(int(stages[i]) + int(b.SelfStageGrowDelta[i])))
			}
		}
	}
	apply(&st.PlayerBuff, &st.PlayerStages)
	apply(&st.EnemyBuff, &st.EnemyStages)
}

func tickCondDot439(st *BattleState) {
	if st == nil {
		return
	}
	run := func(selfBuff *battleBuff, selfStatus *battleStatus, selfStages *[5]int8, foeHP *uint32) {
		if selfBuff == nil || selfBuff.CondDotRounds == 0 || selfBuff.CondDotFlat == 0 || foeHP == nil {
			return
		}
		if hasAnyAbnormalStatus(selfStatus) || hasNegativeStages(selfStages) {
			_ = applyDamage(foeHP, selfBuff.CondDotFlat)
		}
	}
	run(&st.PlayerBuff, &st.PlayerStatus, &st.PlayerStages, &st.EnemyHP)
	run(&st.EnemyBuff, &st.EnemyStatus, &st.EnemyStages, &st.PlayerHP)
}

// applyOnHitAttackerBuffs 进攻方持续效果：104 衰弱 / 109 冻伤 / 116 先手吸血 / 117 先手害怕 / 441 暴击叠层。
func applyOnHitAttackerBuffs(st *BattleState, d *tableloader.SkillDef, playerIsAtk, wentFirst bool, lost uint32) {
	if st == nil || lost == 0 {
		return
	}
	atk := pickSelfBuff(st, playerIsAtk)
	foeStatus := pickFoeStatus(st, playerIsAtk)
	foeStages := pickFoeStages(st, playerIsAtk)
	if atk.OnHitWeakRounds > 0 && int(atk.OnHitWeakChance) > rand.Intn(100) {
		if !stageDropImmuneFromBuff(pickFoeBuff(st, playerIsAtk)) {
			applyWeaknessLayer(foeStages)
		}
	}
	if atk.OnHitFreezeAtkRounds > 0 && int(atk.OnHitFreezeAtkChance) > rand.Intn(100) {
		if !statusImmuneFromBuff(pickFoeBuff(st, playerIsAtk)) {
			setStatusByTableIndex(foeStatus, 5)
		}
	}
	if wentFirst && atk.FirstVampRounds > 0 {
		heal := lost / 5
		if heal < 1 {
			heal = 1
		}
		if playerIsAtk {
			applyHealCap(&st.PlayerHP, &st.PlayerMaxHP, heal)
		} else {
			applyHealCap(&st.EnemyHP, &st.EnemyMaxHP, heal)
		}
	}
	if wentFirst && atk.FirstFearRounds > 0 && rand.Intn(100) < 50 {
		if !statusImmuneFromBuff(pickFoeBuff(st, playerIsAtk)) {
			setStatusByTableIndex(foeStatus, 6)
		}
	}
	if atk.CritStackRounds > 0 && atk.CritStackStep > 0 {
		n := int(atk.CritStackCur) + int(atk.CritStackStep)
		if n > int(atk.CritStackMax) {
			n = int(atk.CritStackMax)
		}
		atk.CritStackCur = byte(n)
	}
	_ = d
}

// applyOnHurtDefenderBuffs 防守方受伤后：128 吸血 / 190 清强化 / 545 超伤异常。
func applyOnHurtDefenderBuffs(st *BattleState, playerIsDef bool, lost uint32) {
	if st == nil || lost == 0 {
		return
	}
	def := pickSelfBuff(st, playerIsDef)
	if def.AbsorbRounds > 0 {
		if playerIsDef {
			applyHealCap(&st.PlayerHP, &st.PlayerMaxHP, lost)
		} else {
			applyHealCap(&st.EnemyHP, &st.EnemyMaxHP, lost)
		}
	}
	if def.OnHurtClearBoostRounds > 0 {
		clearPositiveStages(pickFoeStages(st, playerIsDef))
	}
	if def.OnHurtStatusRounds > 0 && lost > def.OnHurtStatusThr {
		foe := pickFoeStatus(st, playerIsDef)
		if !statusImmuneFromBuff(pickFoeBuff(st, playerIsDef)) {
			setStatusByTableIndex(foe, int(def.OnHurtStatusIdx))
		}
	}
}

// applyOnDodgeBoost 躲避成功时（进攻未命中）：110。
func applyOnDodgeBoost(st *BattleState, defenderIsPlayer bool) {
	if st == nil {
		return
	}
	def := pickSelfBuff(st, defenderIsPlayer)
	if def.OnDodgeBoostRounds == 0 || int(def.OnDodgeBoostChance) <= rand.Intn(100) {
		return
	}
	stat := int(def.OnDodgeBoostStat)
	if stat < 0 || stat > stageSpd {
		stat = stageAtk
	}
	stages := pickSelfStages(st, defenderIsPlayer)
	stages[stat] = int8(clampStage(int(stages[stat]) + 1))
}

// attrSkillBlocked 478：被施加方属性技无效（debuff 在自身 buff 上）。
func attrSkillBlocked(st *BattleState, casterIsPlayer bool, d *tableloader.SkillDef) bool {
	if st == nil || d == nil || d.Category != 4 {
		return false
	}
	self := pickSelfBuff(st, casterIsPlayer)
	return self != nil && self.BlockAttrSkillRounds > 0
}

// critExtraFromStack 441：叠层暴击额外点数。
func critExtraFromStack(b *battleBuff) int {
	if b == nil || b.CritStackRounds == 0 {
		return 0
	}
	// CritStackCur 为百分比文案；折合 /16 制：每 6% ≈ 1 点
	return int(b.CritStackCur) / 6
}
