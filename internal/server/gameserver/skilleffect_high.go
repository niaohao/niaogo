package gameserver

import (
	"math/rand"

	"niaohao/server/internal/tableloader"
)

// statusByTableIndex SkillXML 状态序号：0麻 1毒 2烧 5冻 6怕 8睡。
func statusByTableIndex(st *battleStatus, idx int) bool {
	if st == nil {
		return false
	}
	switch idx {
	case 0:
		return st.Para
	case 1:
		return st.Poison
	case 2:
		return st.Burn
	case 5:
		return st.Freeze
	case 6:
		return st.Fear
	case 7:
		return st.Tired
	case 8:
		return st.Sleep
	default:
		return false
	}
}

func setStatusByTableIndex(st *battleStatus, idx int) {
	if st == nil {
		return
	}
	switch idx {
	case 0:
		setStatus(st, 10)
	case 1:
		setStatus(st, 11)
	case 2:
		setStatus(st, 12)
	case 5:
		setStatus(st, 14)
	case 6:
		setStatus(st, 15)
	case 7:
		setStatus(st, 20)
	case 8:
		setStatus(st, 16)
	}
}

// sideEffectSecondStrikeFlat SideEffect 402：后出手附加固定伤害。
func sideEffectSecondStrikeFlat(d *tableloader.SkillDef, wentFirst bool) uint32 {
	if d == nil || wentFirst || !skillHasSideEffect(d, 402) {
		return 0
	}
	args := parseSideEffectArgs(d.SideEffectArg)
	if len(args) >= 1 && args[0] > 0 {
		return uint32(args[0])
	}
	return 0
}

// sideEffectStatusFlatBonus SideEffect 133/141：烧伤/冻伤附加固伤。
func sideEffectStatusFlatBonus(d *tableloader.SkillDef, foe *battleStatus) uint32 {
	if d == nil || foe == nil {
		return 0
	}
	args := parseSideEffectArgs(d.SideEffectArg)
	flat := 0
	if len(args) >= 1 && args[0] > 0 {
		flat = args[0]
	}
	if flat < 1 {
		return 0
	}
	if skillHasSideEffect(d, 133) && foe.Burn {
		return uint32(flat)
	}
	if skillHasSideEffect(d, 141) && foe.Freeze {
		return uint32(flat)
	}
	return 0
}

// sideEffectDrainPercent SideEffect 101：伤害的 n% 回血。
func sideEffectDrainPercent(d *tableloader.SkillDef, lost uint32) uint32 {
	if d == nil || lost == 0 || !skillHasSideEffect(d, 101) {
		return 0
	}
	args := parseSideEffectArgs(d.SideEffectArg)
	pct := 20
	if len(args) >= 1 && args[0] > 0 {
		pct = args[0]
	}
	if pct > 100 {
		pct = 100
	}
	heal := lost * uint32(pct) / 100
	if heal < 1 {
		heal = 1
	}
	return heal
}

// sideEffectDrainDenom SideEffect 105：伤害的 1/n 回血。
func sideEffectDrainDenom(d *tableloader.SkillDef, lost uint32) uint32 {
	if d == nil || lost == 0 || !skillHasSideEffect(d, 105) {
		return 0
	}
	args := parseSideEffectArgs(d.SideEffectArg)
	denom := 8
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
	return heal
}

// sideEffectCondDrain SideEffect 154：对手特定异常时，伤害 1/n 回血。
func sideEffectCondDrain(d *tableloader.SkillDef, foe *battleStatus, lost uint32) uint32 {
	if d == nil || foe == nil || lost == 0 || !skillHasSideEffect(d, 154) {
		return 0
	}
	args := parseSideEffectArgs(d.SideEffectArg)
	idx, denom := 1, 2
	if len(args) >= 1 {
		idx = args[0]
	}
	if len(args) >= 2 && args[1] > 0 {
		denom = args[1]
	}
	if !statusByTableIndex(foe, idx) {
		return 0
	}
	if denom < 1 {
		denom = 1
	}
	heal := lost / uint32(denom)
	if heal < 1 {
		heal = 1
	}
	return heal
}

// applyHealCap 回血并封顶。
func applyHealCap(hp, max *uint32, heal uint32) {
	if hp == nil || max == nil || heal == 0 {
		return
	}
	*hp += heal
	if *hp > *max {
		*hp = *max
	}
}

// foeParaDamageMul SideEffect 102：对手麻痹伤害翻倍。
func foeParaDamageMul(d *tableloader.SkillDef, foe *battleStatus, dmg uint32) uint32 {
	if d == nil || foe == nil || dmg == 0 || !skillHasSideEffect(d, 102) || !foe.Para {
		return dmg
	}
	return dmg * 2
}

// sameTypePowerBoost SideEffect 179：技能属性与自身相同则伤害 ×(100+n)/100。
func sameTypePowerBoost(d *tableloader.SkillDef, selfType int, dmg uint32) uint32 {
	if d == nil || dmg == 0 || !skillHasSideEffect(d, 179) || d.Type != selfType {
		return dmg
	}
	args := parseSideEffectArgs(d.SideEffectArg)
	n := 20
	if len(args) >= 1 && args[0] > 0 {
		n = args[0]
	}
	if n > 500 {
		n = 500
	}
	return dmg * uint32(100+n) / 100
}

// genderTargetDamageMul SideEffect 82：雄性×2，雌性×0.5。
func genderTargetDamageMul(d *tableloader.SkillDef, foeGender int, dmg uint32) uint32 {
	if d == nil || dmg == 0 || !skillHasSideEffect(d, 82) {
		return dmg
	}
	switch foeGender {
	case 1:
		return dmg * 2
	case 2:
		return dmg / 2
	default:
		return dmg
	}
}

// clearFoeOngoingBuffs SideEffect 180：清除对手回合类持续效果。
func clearFoeOngoingBuffs(st *BattleState, playerIsAtk bool) {
	if st == nil {
		return
	}
	*pickFoeBuff(st, playerIsAtk) = battleBuff{}
	if playerIsAtk {
		st.EnemyChargeSkill = 0
		st.EnemyChargeReady = false
		st.EnemyDoomRounds = 0
		st.EnemySkillFail = false
		st.EnemyCritBonusRounds = 0
		st.EnemyTypeOverrideRounds = 0
	} else {
		st.PlayerChargeSkill = 0
		st.PlayerChargeReady = false
		st.PlayerDoomRounds = 0
		st.PlayerSkillFail = false
		st.PlayerCritBonusRounds = 0
		st.PlayerTypeOverrideRounds = 0
	}
}

// applySecondStrikeStage SideEffect 148（改敌）/186（改己）：后手概率改能力。
func applySecondStrikeStage(st *BattleState, d *tableloader.SkillDef, eid int, playerIsAtk, wentFirst bool, args []int, argOff int) int {
	stat, chance, delta := 0, 50, 1
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
	if st == nil || d == nil || wentFirst || !skillHasSideEffect(d, eid) {
		return argOff
	}
	if chance > 100 {
		chance = 100
	}
	if stat < 0 || stat > stageSpd || rand.Intn(100) >= chance {
		return argOff
	}
	var stages *[5]int8
	if eid == 148 {
		stages = pickFoeStages(st, playerIsAtk)
		if stageDropImmuneFromBuff(pickFoeBuff(st, playerIsAtk)) && delta < 0 {
			return argOff
		}
	} else {
		stages = pickSelfStages(st, playerIsAtk)
	}
	if stages == nil {
		return argOff
	}
	stages[stat] = int8(clampStage(int(stages[stat]) + delta))
	return argOff
}

// applySecondStrikeStatus SideEffect 147：后手概率上异常。
func applySecondStrikeStatus(st *BattleState, d *tableloader.SkillDef, playerIsAtk, wentFirst bool, args []int, argOff int) int {
	if st == nil || d == nil || !skillHasSideEffect(d, 147) {
		return argOff
	}
	chance, idx := 30, 0
	if argOff < len(args) {
		chance = args[argOff]
		argOff++
	}
	if argOff < len(args) {
		idx = args[argOff]
		argOff++
	}
	if chance > 100 {
		chance = 100
	}
	if wentFirst || rand.Intn(100) >= chance {
		return argOff
	}
	foe := pickFoeStatus(st, playerIsAtk)
	if statusImmuneFromBuff(pickFoeBuff(st, playerIsAtk)) {
		return argOff
	}
	setStatusByTableIndex(foe, idx)
	return argOff
}

// applyLowDamageSelfBoost SideEffect 107：实际伤害 < 阈值则自身某维 +1。
func applyLowDamageSelfBoost(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, actual uint32) {
	if st == nil || d == nil || !skillHasSideEffect(d, 107) {
		return
	}
	args := parseSideEffectArgs(d.SideEffectArg)
	thr, stat := 100, 0
	if len(args) >= 1 && args[0] > 0 {
		thr = args[0]
	}
	if len(args) >= 2 {
		stat = args[1]
	}
	if actual == 0 || int(actual) >= thr || stat < 0 || stat > stageSpd {
		return
	}
	stages := pickSelfStages(st, playerIsAtk)
	stages[stat] = int8(clampStage(int(stages[stat]) + 1))
}

// applyOnKOSelfStage SideEffect 158：击杀时概率自身能力上升。
func applyOnKOSelfStage(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, foeHPBefore uint32) {
	if st == nil || d == nil || foeHPBefore == 0 || !skillHasSideEffect(d, 158) {
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
	args := parseSideEffectArgs(d.SideEffectArg)
	stat, chance, delta := 0, 100, 1
	if len(args) >= 1 {
		stat = args[0]
	}
	if len(args) >= 2 && args[1] > 0 {
		chance = args[1]
	}
	if len(args) >= 3 && args[2] != 0 {
		delta = args[2]
	}
	if chance > 100 {
		chance = 100
	}
	if stat < 0 || stat > stageSpd || rand.Intn(100) >= chance {
		return
	}
	stages := pickSelfStages(st, playerIsAtk)
	stages[stat] = int8(clampStage(int(stages[stat]) + delta))
}

// applyGenderSelfBuff SideEffect 83：雄→先手标记；雌→必暴。
func applyGenderSelfBuff(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, selfGender int) {
	if st == nil || d == nil || !skillHasSideEffect(d, 83) {
		return
	}
	self := pickSelfBuff(st, playerIsAtk)
	switch selfGender {
	case 1:
		self.PriorityRounds = 2
	case 2:
		self.MustCritRounds = 2
	}
}

// mirrorSyncChanges SideEffect 91：若一侧有同步标记，把对另一侧的变化镜像过来。
func mirrorSyncChanges(st *BattleState, changedIsPlayer bool, apply func(targetIsPlayer bool)) {
	if st == nil || apply == nil {
		return
	}
	if changedIsPlayer && st.EnemyBuff.SyncChangesRounds > 0 {
		apply(false)
	}
	if !changedIsPlayer && st.PlayerBuff.SyncChangesRounds > 0 {
		apply(true)
	}
}

// applyHighDamageSideEffects SideEffect 82/102/129–131/133/141/162/168/179/402：伤害倍率与附加固伤。
func applyHighDamageSideEffects(d *tableloader.SkillDef, dmg uint32, foe *battleStatus, selfType, foeGender int, wentFirst bool) uint32 {
	if d == nil {
		return dmg
	}
	if genderMatchImmune(d, foeGender) {
		return 0
	}
	if dmg > 0 {
		dmg = foeParaDamageMul(d, foe, dmg)
		dmg = sameTypePowerBoost(d, selfType, dmg)
		dmg = genderTargetDamageMul(d, foeGender, dmg)
		dmg = genderMatchPowerMul(d, foeGender, dmg)
		dmg = foeSleepDamageMul(d, foe, dmg)
	}
	dmg += sideEffectStatusFlatBonus(d, foe)
	dmg += sideEffectSecondStrikeFlat(d, wentFirst)
	dmg += genderMatchFlat(d, foeGender)
	dmg += sideEffectAnyStatusFlat(d, foe)
	return dmg
}

// priorityFromBuff SideEffect 83：强制先手（仅一侧有标记时生效）。
func priorityFromBuff(self, foe *battleBuff) (forcedFirst, forcedSecond bool) {
	selfP := self != nil && self.PriorityRounds > 0
	foeP := foe != nil && foe.PriorityRounds > 0
	if selfP && !foeP {
		return true, false
	}
	if foeP && !selfP {
		return false, true
	}
	return false, false
}
