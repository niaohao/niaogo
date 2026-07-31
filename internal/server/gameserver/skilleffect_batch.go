package gameserver

import (
	"math/rand"

	"niaohao/server/internal/tableloader"
)

// —— 表频≈2 且有 Effect_*.as 的一批 SideEffect（2026-07-30）——

func hasAnyAbnormalStatus(st *battleStatus) bool {
	if st == nil {
		return false
	}
	return st.Para || st.Poison || st.Burn || st.Freeze || st.Fear || st.Tired || st.Sleep
}

// sideEffectDamagePercentFlat SideEffect 422：附加所造成伤害值 X% 的固定伤害。
func sideEffectDamagePercentFlat(d *tableloader.SkillDef, dmg uint32) uint32 {
	if d == nil || dmg == 0 || !skillHasSideEffect(d, 422) {
		return 0
	}
	args := sideEffectArgsFor(d, 422)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	pct := 0
	if len(args) >= 1 {
		pct = args[0]
	}
	if pct <= 0 {
		return 0
	}
	extra := dmg * uint32(pct) / 100
	if extra < 1 {
		extra = 1
	}
	return extra
}

// sideEffectLostHPPercentFlat SideEffect 436：附加已损失体力 X% 的固定伤害。
func sideEffectLostHPPercentFlat(d *tableloader.SkillDef, selfHP, selfMaxHP uint32) uint32 {
	if d == nil || selfMaxHP == 0 || !skillHasSideEffect(d, 436) {
		return 0
	}
	args := sideEffectArgsFor(d, 436)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	pct := 30
	if len(args) >= 1 && args[0] > 0 {
		pct = args[0]
	}
	lost := uint32(0)
	if selfHP < selfMaxHP {
		lost = selfMaxHP - selfHP
	}
	if lost == 0 {
		return 0
	}
	return lost * uint32(pct) / 100
}

// sideEffectLostHPChunkFlat SideEffect 455：每损失 n 点体力附加 m 点固伤。
func sideEffectLostHPChunkFlat(d *tableloader.SkillDef, selfHP, selfMaxHP uint32) uint32 {
	if d == nil || selfMaxHP == 0 || !skillHasSideEffect(d, 455) {
		return 0
	}
	args := sideEffectArgsFor(d, 455)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	n, m := 1, 1
	if len(args) >= 1 && args[0] > 0 {
		n = args[0]
	}
	if len(args) >= 2 && args[1] > 0 {
		m = args[1]
	}
	lost := uint32(0)
	if selfHP < selfMaxHP {
		lost = selfMaxHP - selfHP
	}
	if lost == 0 || n < 1 {
		return 0
	}
	return (lost / uint32(n)) * uint32(m)
}

// sideEffectTypeAdvantageFlat SideEffect 428：遇到天敌（属性克制）附加固伤。
func sideEffectTypeAdvantageFlat(d *tableloader.SkillDef, skillType, foeType int) uint32 {
	if d == nil || !skillHasSideEffect(d, 428) {
		return 0
	}
	if typeMultiplier(skillType, foeType) <= 1 {
		return 0
	}
	args := sideEffectArgsFor(d, 428)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	if len(args) >= 1 && args[0] > 0 {
		return uint32(args[0])
	}
	return 0
}

// genderMatchPowerMul SideEffect 129：对方为指定性别则威力翻倍。
func genderMatchPowerMul(d *tableloader.SkillDef, foeGender int, dmg uint32) uint32 {
	if d == nil || dmg == 0 || !skillHasSideEffect(d, 129) {
		return dmg
	}
	args := sideEffectArgsFor(d, 129)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	want := 1
	if len(args) >= 1 {
		want = args[0]
	}
	if foeGender == want {
		return dmg * 2
	}
	return dmg
}

// genderMatchFlat SideEffect 130：对方为指定性别则附加固伤。
func genderMatchFlat(d *tableloader.SkillDef, foeGender int) uint32 {
	if d == nil || !skillHasSideEffect(d, 130) {
		return 0
	}
	args := sideEffectArgsFor(d, 130)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	want, flat := 1, 0
	if len(args) >= 1 {
		want = args[0]
	}
	if len(args) >= 2 && args[1] > 0 {
		flat = args[1]
	}
	if foeGender != want || flat <= 0 {
		return 0
	}
	return uint32(flat)
}

// mustCritFromSideEffect193 SideEffect 193：对手处于指定异常则必定暴击。
func mustCritFromSideEffect193(d *tableloader.SkillDef, foe *battleStatus) bool {
	if d == nil || foe == nil || !skillHasSideEffect(d, 193) {
		return false
	}
	args := sideEffectArgsFor(d, 193)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	idx := 5 // 默认冻伤
	if len(args) >= 1 {
		idx = args[0]
	}
	return statusByTableIndexEx(foe, idx)
}

// applyFreq2DamageSideEffects SideEffect 422/436/455/428。
func applyFreq2DamageSideEffects(d *tableloader.SkillDef, dmg, selfHP, selfMaxHP uint32, skillType, foeType int) uint32 {
	if d == nil {
		return dmg
	}
	dmg += sideEffectDamagePercentFlat(d, dmg)
	dmg += sideEffectLostHPPercentFlat(d, selfHP, selfMaxHP)
	dmg += sideEffectLostHPChunkFlat(d, selfHP, selfMaxHP)
	dmg += sideEffectTypeAdvantageFlat(d, skillType, foeType)
	return dmg
}

// applySecondStrikeDrain SideEffect 172：后出手则回复造成伤害的 1/n。
func applySecondStrikeDrain(st *BattleState, d *tableloader.SkillDef, playerIsAtk, wentFirst bool, lost uint32) {
	if st == nil || d == nil || wentFirst || lost == 0 || !skillHasSideEffect(d, 172) {
		return
	}
	args := sideEffectArgsFor(d, 172)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	denom := 3
	if len(args) >= 1 && args[0] > 0 {
		denom = args[0]
	}
	if denom < 1 {
		denom = 1
	}
	heal := lost / uint32(denom)
	if heal < 1 {
		heal = 1
	}
	if playerIsAtk {
		applyHealCap(&st.PlayerHP, &st.PlayerMaxHP, heal)
	} else {
		applyHealCap(&st.EnemyHP, &st.EnemyMaxHP, heal)
	}
}

// applyFirstStrikeStatus SideEffect 173：先出手时概率令对方异常。
func applyFirstStrikeStatus(st *BattleState, d *tableloader.SkillDef, playerIsAtk, wentFirst bool, args []int, argOff int) int {
	chance, idx := 30, 0
	if argOff < len(args) {
		chance = args[argOff]
		argOff++
	}
	if argOff < len(args) {
		idx = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !wentFirst || !skillHasSideEffect(d, 173) {
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

// applyFirstStrikeSelfStage SideEffect 474：先出手时概率自身能力上升。
func applyFirstStrikeSelfStage(st *BattleState, d *tableloader.SkillDef, playerIsAtk, wentFirst bool, args []int, argOff int) int {
	stat, chance, delta := 0, 100, 1
	if argOff < len(args) {
		stat = args[argOff]
		argOff++
	}
	if argOff < len(args) {
		chance = args[argOff]
		argOff++
	}
	if argOff < len(args) {
		delta = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !wentFirst || !skillHasSideEffect(d, 474) {
		return argOff
	}
	if chance > 100 {
		chance = 100
	}
	if stat < 0 || stat > stageSpd || rand.Intn(100) >= chance {
		return argOff
	}
	stages := pickSelfStages(st, playerIsAtk)
	stages[stat] = int8(clampStage(int(stages[stat]) + delta))
	return argOff
}

// applyClearBoostSelfStage SideEffect 430：消除对手能力强化，成功则自身能力变化。
func applyClearBoostSelfStage(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	stat, delta := 0, 1
	if argOff < len(args) {
		stat = args[argOff]
		argOff++
	}
	if argOff < len(args) {
		delta = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 430) {
		return argOff
	}
	foe := pickFoeStages(st, playerIsAtk)
	if !hasPositiveStages(foe) {
		return argOff
	}
	clearPositiveStages(foe)
	if stat < 0 || stat > stageSpd {
		return argOff
	}
	self := pickSelfStages(st, playerIsAtk)
	self[stat] = int8(clampStage(int(self[stat]) + delta))
	return argOff
}

// applyLowDamageSelfBoostEx SideEffect 473：伤害不足阈值则自身能力上升（可指定级数）。
func applyLowDamageSelfBoostEx(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, actual uint32) {
	if st == nil || d == nil || !skillHasSideEffect(d, 473) {
		return
	}
	args := sideEffectArgsFor(d, 473)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	thr, stat, delta := 300, 0, 2
	if len(args) >= 1 && args[0] > 0 {
		thr = args[0]
	}
	if len(args) >= 2 {
		stat = args[1]
	}
	if len(args) >= 3 && args[2] != 0 {
		delta = args[2]
	}
	if actual == 0 || int(actual) >= thr || stat < 0 || stat > stageSpd {
		return
	}
	stages := pickSelfStages(st, playerIsAtk)
	stages[stat] = int8(clampStage(int(stages[stat]) + delta))
}

// applyCondStatusSelfStage SideEffect 175：对手有异常时概率自身能力变化。
func applyCondStatusSelfStage(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	stat, chance, delta := 0, 100, 1
	if argOff < len(args) {
		stat = args[argOff]
		argOff++
	}
	if argOff < len(args) {
		chance = args[argOff]
		argOff++
	}
	if argOff < len(args) {
		delta = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 175) {
		return argOff
	}
	if chance > 100 {
		chance = 100
	}
	if !hasAnyAbnormalStatus(pickFoeStatus(st, playerIsAtk)) || stat < 0 || stat > stageSpd || rand.Intn(100) >= chance {
		return argOff
	}
	stages := pickSelfStages(st, playerIsAtk)
	stages[stat] = int8(clampStage(int(stages[stat]) + delta))
	return argOff
}

// applyCondBoostSelfStage SideEffect 184：对手有能力强化时概率自身能力上升。
func applyCondBoostSelfStage(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	stat, chance, delta := 0, 100, 1
	if argOff < len(args) {
		stat = args[argOff]
		argOff++
	}
	if argOff < len(args) {
		chance = args[argOff]
		argOff++
	}
	if argOff < len(args) {
		delta = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 184) {
		return argOff
	}
	if chance > 100 {
		chance = 100
	}
	if !hasPositiveStages(pickFoeStages(st, playerIsAtk)) || stat < 0 || stat > stageSpd || rand.Intn(100) >= chance {
		return argOff
	}
	stages := pickSelfStages(st, playerIsAtk)
	stages[stat] = int8(clampStage(int(stages[stat]) + delta))
	return argOff
}

// —— 续补批（2026-07-30 #2）——

// genderMatchImmune SideEffect 131：对方为指定性别则本回合伤害免疫（归零）。
func genderMatchImmune(d *tableloader.SkillDef, foeGender int) bool {
	if d == nil || !skillHasSideEffect(d, 131) {
		return false
	}
	args := sideEffectArgsFor(d, 131)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	want := 1
	if len(args) >= 1 {
		want = args[0]
	}
	return foeGender == want
}

// sideEffectAnyStatusFlat SideEffect 162：对手异常则附加固伤。
func sideEffectAnyStatusFlat(d *tableloader.SkillDef, foe *battleStatus) uint32 {
	if d == nil || !skillHasSideEffect(d, 162) || !hasAnyAbnormalStatus(foe) {
		return 0
	}
	args := sideEffectArgsFor(d, 162)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	if len(args) >= 1 && args[0] > 0 {
		return uint32(args[0])
	}
	return 0
}

// foeSleepDamageMul SideEffect 168：对手睡眠则威力翻倍。
func foeSleepDamageMul(d *tableloader.SkillDef, foe *battleStatus, dmg uint32) uint32 {
	if d == nil || dmg == 0 || foe == nil || !skillHasSideEffect(d, 168) || !foe.Sleep {
		return dmg
	}
	return dmg * 2
}

// foeDropDamageMul SideEffect 431：对手能力下降则威力翻倍。
func foeDropDamageMul(d *tableloader.SkillDef, foeStages *[5]int8, dmg uint32) uint32 {
	if d == nil || dmg == 0 || !skillHasSideEffect(d, 431) || !hasNegativeStages(foeStages) {
		return dmg
	}
	return dmg * 2
}

// applyTypeAdvantageBurn SideEffect 464：天敌时概率烧伤。
func applyTypeAdvantageBurn(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, skillType, foeType int) {
	if st == nil || d == nil || !skillHasSideEffect(d, 464) {
		return
	}
	if typeMultiplier(skillType, foeType) <= 1 {
		return
	}
	args := sideEffectArgsFor(d, 464)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	chance := 100
	if len(args) >= 1 && args[0] > 0 {
		chance = args[0]
	}
	if chance > 100 {
		chance = 100
	}
	if rand.Intn(100) >= chance {
		return
	}
	foe := pickFoeStatus(st, playerIsAtk)
	if statusImmuneFromBuff(pickFoeBuff(st, playerIsAtk)) {
		return
	}
	setStatusByTableIndex(foe, 2) // 烧伤
}

// applySelfBoostStatus SideEffect 434：自身有强化时概率令对手异常。
func applySelfBoostStatus(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	chance, idx := 50, 0
	if argOff < len(args) {
		chance = args[argOff]
		argOff++
	}
	if argOff < len(args) {
		idx = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 434) {
		return argOff
	}
	if !hasPositiveStages(pickSelfStages(st, playerIsAtk)) {
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

// applyClearBoostFoeStatus SideEffect 453：消除对手强化，成功则令其异常。
func applyClearBoostFoeStatus(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	idx := 0
	if argOff < len(args) {
		idx = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 453) {
		return argOff
	}
	foeStages := pickFoeStages(st, playerIsAtk)
	if !hasPositiveStages(foeStages) {
		return argOff
	}
	clearPositiveStages(foeStages)
	foe := pickFoeStatus(st, playerIsAtk)
	if statusImmuneFromBuff(pickFoeBuff(st, playerIsAtk)) {
		return argOff
	}
	setStatusByTableIndex(foe, idx)
	return argOff
}

// applyChanceMaxHPHeal SideEffect 438/410：n% 几率恢复自身体力 1/m。
func applyChanceMaxHPHeal(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	chance, denom := 50, 4
	if argOff < len(args) {
		chance = args[argOff]
		argOff++
	}
	if argOff < len(args) && args[argOff] > 0 {
		denom = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !(skillHasSideEffect(d, 438) || skillHasSideEffect(d, 410)) {
		return argOff
	}
	if chance > 100 {
		chance = 100
	}
	if denom < 1 {
		denom = 1
	}
	if rand.Intn(100) >= chance {
		return argOff
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

// applySameTypeDrain SideEffect 178：伤害 1/n 回血；属性相同时 1/m。
func applySameTypeDrain(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, lost uint32, selfType int) {
	if st == nil || d == nil || lost == 0 || !skillHasSideEffect(d, 178) {
		return
	}
	args := sideEffectArgsFor(d, 178)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	denom, sameDenom := 4, 2
	if len(args) >= 1 && args[0] > 0 {
		denom = args[0]
	}
	if len(args) >= 2 && args[1] > 0 {
		sameDenom = args[1]
	}
	use := denom
	if d.Type > 0 && d.Type == selfType {
		use = sameDenom
	}
	if use < 1 {
		use = 1
	}
	heal := lost / uint32(use)
	if heal < 1 {
		heal = 1
	}
	if playerIsAtk {
		applyHealCap(&st.PlayerHP, &st.PlayerMaxHP, heal)
	} else {
		applyHealCap(&st.EnemyHP, &st.EnemyMaxHP, heal)
	}
}

// applyCondStatusDrainEx SideEffect 194：伤害 1/n 回血；对手特定异常则 1/m。
func applyCondStatusDrainEx(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, lost uint32) {
	if st == nil || d == nil || lost == 0 || !skillHasSideEffect(d, 194) {
		return
	}
	args := sideEffectArgsFor(d, 194)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	denom, idx, condDenom := 4, 5, 2
	if len(args) >= 1 && args[0] > 0 {
		denom = args[0]
	}
	if len(args) >= 2 {
		idx = args[1]
	}
	if len(args) >= 3 && args[2] > 0 {
		condDenom = args[2]
	}
	use := denom
	if statusByTableIndexEx(pickFoeStatus(st, playerIsAtk), idx) {
		use = condDenom
	}
	if use < 1 {
		use = 1
	}
	heal := lost / uint32(use)
	if heal < 1 {
		heal = 1
	}
	if playerIsAtk {
		applyHealCap(&st.PlayerHP, &st.PlayerMaxHP, heal)
	} else {
		applyHealCap(&st.EnemyHP, &st.EnemyMaxHP, heal)
	}
}

// applyLowDamagePPBoost SideEffect 134：伤害低于阈值则所有技能 PP +n。
func applyLowDamagePPBoost(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, actual uint32, skillOf func(int) *tableloader.SkillDef) {
	if st == nil || d == nil || !skillHasSideEffect(d, 134) {
		return
	}
	args := sideEffectArgsFor(d, 134)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	thr, add := 100, 1
	if len(args) >= 1 && args[0] > 0 {
		thr = args[0]
	}
	if len(args) >= 2 && args[1] > 0 {
		add = args[1]
	}
	if actual == 0 || int(actual) >= thr {
		return
	}
	skills := st.PlayerSkills
	if !playerIsAtk {
		skills = st.EnemySkills
	}
	for i := range skills {
		sid := int(skills[i][0])
		if sid == 0 {
			continue
		}
		maxPP := uint32(0)
		if skillOf != nil {
			if def := skillOf(sid); def != nil && def.MaxPP > 0 {
				maxPP = uint32(def.MaxPP)
			}
		}
		skills[i][1] += uint32(add)
		if maxPP > 0 && skills[i][1] > maxPP {
			skills[i][1] = maxPP
		}
	}
}
