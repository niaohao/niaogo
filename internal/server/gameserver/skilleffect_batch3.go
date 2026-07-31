package gameserver

import (
	"math/rand"

	"niaohao/server/internal/tableloader"
)

// —— 续补批（2026-07-30 #3，freq≈1 + 有 AS）——

// sideEffectLevelFlat SideEffect 111：附加 2×自身等级固伤。
func sideEffectLevelFlat(d *tableloader.SkillDef, level int) uint32 {
	if d == nil || level <= 0 || !skillHasSideEffect(d, 111) {
		return 0
	}
	return uint32(level * 2)
}

// sideEffectSpdChanceFlat SideEffect 115：15% 附加速度 1/3 固伤。
func sideEffectSpdChanceFlat(d *tableloader.SkillDef, spd int) uint32 {
	if d == nil || spd <= 0 || !skillHasSideEffect(d, 115) {
		return 0
	}
	if rand.Intn(100) >= 15 {
		return 0
	}
	flat := spd / 3
	if flat < 1 {
		flat = 1
	}
	return uint32(flat)
}

// sideEffectRandomFlat139 SideEffect 139：随机固伤替换（50%:301–350 / 30%:101–300 / 20%:5–100）。
func sideEffectRandomFlat139(d *tableloader.SkillDef) (uint32, bool) {
	if d == nil || !skillHasSideEffect(d, 139) {
		return 0, false
	}
	r := rand.Intn(100)
	switch {
	case r < 50:
		return uint32(301 + rand.Intn(50)), true
	case r < 80:
		return uint32(101 + rand.Intn(200)), true
	default:
		return uint32(5 + rand.Intn(96)), true
	}
}

// sideEffectFoeHPPercentFlat SideEffect 411：附加对手当前体力 n% 固伤，连续使用递增，最高 m%。
func sideEffectFoeHPPercentFlat(d *tableloader.SkillDef, foeHP uint32, consec uint32) uint32 {
	if d == nil || foeHP == 0 || !skillHasSideEffect(d, 411) {
		return 0
	}
	args := sideEffectArgsFor(d, 411)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	base, step, maxPct := 8, 0, 8
	if len(args) >= 1 && args[0] > 0 {
		base = args[0]
	}
	if len(args) >= 2 && args[1] > 0 {
		step = args[1]
	}
	if len(args) >= 3 && args[2] > 0 {
		maxPct = args[2]
	}
	pct := base + int(consec)*step
	if pct > maxPct {
		pct = maxPct
	}
	if pct < 1 {
		return 0
	}
	return foeHP * uint32(pct) / 100
}

// lowHPVsFoeDamageMul SideEffect 132：自身 HP 低于对手则伤害翻倍。
func lowHPVsFoeDamageMul(d *tableloader.SkillDef, selfHP, foeHP, dmg uint32) uint32 {
	if d == nil || dmg == 0 || !skillHasSideEffect(d, 132) || selfHP >= foeHP {
		return dmg
	}
	return dmg * 2
}

// samePetTypeDamageMul SideEffect 401：与对手属性相同则伤害翻倍。
func samePetTypeDamageMul(d *tableloader.SkillDef, selfType, foeType int, dmg uint32) uint32 {
	if d == nil || dmg == 0 || !skillHasSideEffect(d, 401) || selfType <= 0 || selfType != foeType {
		return dmg
	}
	return dmg * 2
}

// sacrificeAllForFlat SideEffect 112：牺牲全部体力，造成 250~300；致死留 1 由 leave-one 处理。
func sacrificeAllForFlat(d *tableloader.SkillDef, selfHP uint32) (dmg, loss uint32, ok bool) {
	if d == nil || !skillHasSideEffect(d, 112) {
		return 0, 0, false
	}
	if selfHP == 0 {
		return 0, 0, true
	}
	return uint32(250 + rand.Intn(51)), selfHP, true
}

// applyFreq3DamageSideEffects SideEffect 111/115/132/139/401/411。
func applyFreq3DamageSideEffects(d *tableloader.SkillDef, dmg, selfHP, foeHP uint32, level, spd, selfType, foeType int, consec uint32) uint32 {
	if d == nil {
		return dmg
	}
	if flat, ok := sideEffectRandomFlat139(d); ok {
		return flat
	}
	if dmg > 0 {
		dmg = lowHPVsFoeDamageMul(d, selfHP, foeHP, dmg)
		dmg = samePetTypeDamageMul(d, selfType, foeType, dmg)
	}
	dmg += sideEffectLevelFlat(d, level)
	dmg += sideEffectSpdChanceFlat(d, spd)
	dmg += sideEffectFoeHPPercentFlat(d, foeHP, consec)
	return dmg
}

// mustCritFromAnyStatus SideEffect 188：对手有异常则必定暴击。
func mustCritFromAnyStatus(d *tableloader.SkillDef, foe *battleStatus) bool {
	return d != nil && skillHasSideEffect(d, 188) && hasAnyAbnormalStatus(foe)
}

// applyOddEvenHitEffect SideEffect 119：伤害奇→30%疲惫；偶→30%速度+1。
func applyOddEvenHitEffect(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, lost uint32) {
	if st == nil || d == nil || lost == 0 || !skillHasSideEffect(d, 119) {
		return
	}
	if lost%2 == 1 {
		if rand.Intn(100) >= 30 {
			return
		}
		foe := pickFoeStatus(st, playerIsAtk)
		if statusImmuneFromBuff(pickFoeBuff(st, playerIsAtk)) {
			return
		}
		setStatusByTableIndex(foe, 7) // 疲惫
		return
	}
	if rand.Intn(100) >= 30 {
		return
	}
	stages := pickSelfStages(st, playerIsAtk)
	stages[stageSpd] = int8(clampStage(int(stages[stageSpd]) + 1))
}

// applySameTypePara SideEffect 121：属性相同时概率麻痹。
func applySameTypePara(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, selfType int, args []int, argOff int) int {
	chance := 30
	if argOff < len(args) {
		chance = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 121) {
		return argOff
	}
	if d.Type <= 0 || d.Type != selfType {
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
	setStatusByTableIndex(foe, 0) // 麻痹
	return argOff
}

// applyRandomStageDrop SideEffect 124：命中后概率随机降一项能力。
func applyRandomStageDrop(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	chance, delta := 30, -1
	if argOff < len(args) {
		chance = args[argOff]
		argOff++
	}
	if argOff < len(args) {
		delta = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 124) {
		return argOff
	}
	if chance > 100 {
		chance = 100
	}
	if rand.Intn(100) >= chance {
		return argOff
	}
	if stageDropImmuneFromBuff(pickFoeBuff(st, playerIsAtk)) && delta < 0 {
		return argOff
	}
	stat := rand.Intn(stageSpd + 1)
	stages := pickFoeStages(st, playerIsAtk)
	stages[stat] = int8(clampStage(int(stages[stat]) + delta))
	return argOff
}

// applyPoisonMaxHPHeal SideEffect 145：对手中毒时回复最大体力 1/n。
func applyPoisonMaxHPHeal(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool) {
	if st == nil || d == nil || !skillHasSideEffect(d, 145) {
		return
	}
	if !pickFoeStatus(st, playerIsAtk).Poison {
		return
	}
	args := sideEffectArgsFor(d, 145)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	denom := 4
	if len(args) >= 1 && args[0] > 0 {
		denom = args[0]
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
}

// applyBurnCondTired SideEffect 151：烧伤/未烧伤不同概率疲惫。
func applyBurnCondTired(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	burnChance, plainChance := 50, 20
	if argOff < len(args) {
		burnChance = args[argOff]
		argOff++
	}
	if argOff < len(args) {
		plainChance = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 151) {
		return argOff
	}
	chance := plainChance
	if pickFoeStatus(st, playerIsAtk).Burn {
		chance = burnChance
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

// applyLowHPStatus SideEffect 159：自身体力小于最大 1/n 时概率异常。
func applyLowHPStatus(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	denom, chance, idx := 2, 50, 0
	if argOff < len(args) && args[argOff] > 0 {
		denom = args[argOff]
		argOff++
	}
	if argOff < len(args) {
		chance = args[argOff]
		argOff++
	}
	if argOff < len(args) {
		idx = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 159) {
		return argOff
	}
	hp, maxHP := st.EnemyHP, st.EnemyMaxHP
	if playerIsAtk {
		hp, maxHP = st.PlayerHP, st.PlayerMaxHP
	}
	if denom < 1 || maxHP == 0 || hp*uint32(denom) >= maxHP {
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

// applyHighDamageHeal SideEffect 415：伤害大于阈值则回复固定体力。
func applyHighDamageHeal(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, lost uint32) {
	if st == nil || d == nil || !skillHasSideEffect(d, 415) {
		return
	}
	args := sideEffectArgsFor(d, 415)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	thr, heal := 100, 50
	if len(args) >= 1 && args[0] > 0 {
		thr = args[0]
	}
	if len(args) >= 2 && args[1] > 0 {
		heal = args[1]
	}
	if int(lost) <= thr {
		return
	}
	if playerIsAtk {
		applyHealCap(&st.PlayerHP, &st.PlayerMaxHP, uint32(heal))
	} else {
		applyHealCap(&st.EnemyHP, &st.EnemyMaxHP, uint32(heal))
	}
}

// applyRandomDotStatus SideEffect 451：命中后概率随机烧/冻/毒。
func applyRandomDotStatus(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	chance := 30
	if argOff < len(args) {
		chance = args[argOff]
		argOff++
	}
	// 第二参部分技能占位，跳过
	if argOff < len(args) {
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 451) {
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
	pool := []int{1, 2, 5} // 毒/烧/冻
	setStatusByTableIndex(foe, pool[rand.Intn(len(pool))])
	return argOff
}
