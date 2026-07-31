package gameserver

import (
	"math/rand"

	"niaohao/server/internal/tableloader"
)

// sideEffectArgCount 对齐本前端 Effect_*.as 的 _argsNum（未知默认 0，不吞参）。
func sideEffectArgCount(eid int) int {
	switch eid {
	case 4, 5:
		return 3 // 能力组：stat chance delta（applyStatGroups 另计）
	case 6, 28, 29, 32, 36, 37, 38, 39, 43, 53, 54, 61, 66, 70, 77, 88, 93, 94, 99,
		101, 103, 105, 106, 114, 120, 121, 128, 129, 131, 133, 135, 136, 141, 145, 156, 162, 167, 172, 179, 185, 190, 193, 202, 402, 405, 412, 413, 422, 428, 436, 447, 453, 456, 459, 461, 464, 466, 471, 472, 476, 478, 508,
		691, 700, 976, 1211, 1257, 1470, 1605, 1925, 2236, 2237:
		return 1
	case 9, 20, 22, 31, 41, 60, 65, 74, 75, 76, 79, 84, 89, 90, 92, 98, 104, 107, 108, 109,
		124, 125, 126, 130, 134, 147, 151, 154, 173, 178, 410, 415, 418, 430, 434, 437, 438, 439, 441, 449, 450, 451, 454, 455, 460, 463, 467, 475, 482, 545, 687, 1635,
		487, 488, 495, 795, 1248, 1603, 1850:
		return 2
	case 55, 56, 110, 122, 123, 148, 158, 159, 175, 184, 186, 194, 411, 429, 473, 474, 484:
		return 3
	case 181, 182, 465:
		return 4
	case 196:
		return 6
	case 433, 448:
		return 7
	case 192:
		return 10
	case 201:
		return 2
	case 69:
		return 5
	case 485, 494, 773, 1083:
		return 0
	case 935:
		return 1
	default:
		return 0
	}
}

// sideEffectArgsFor 按 SideEffect 顺序切出指定 eid 的参数；456 若在末尾且切空则回落末参。
func sideEffectArgsFor(d *tableloader.SkillDef, eid int) []int {
	if d == nil {
		return nil
	}
	ids := parseSideEffectIDs(d.SideEffect)
	args := parseSideEffectArgs(d.SideEffectArg)
	off := 0
	for i, id := range ids {
		n := sideEffectArgCount(id)
		if id == eid {
			if n <= 0 {
				break
			}
			end := off + n
			if off >= len(args) {
				break
			}
			if end > len(args) {
				end = len(args)
			}
			return args[off:end]
		}
		off += n
		_ = i
	}
	if eid == 456 && len(ids) > 0 && ids[len(ids)-1] == 456 && len(args) > 0 {
		return []int{args[len(args)-1]}
	}
	return nil
}

func hasPositiveStages(stages *[5]int8) bool {
	if stages == nil {
		return false
	}
	for _, v := range stages {
		if v > 0 {
			return true
		}
	}
	return false
}

func hasNegativeStages(stages *[5]int8) bool {
	if stages == nil {
		return false
	}
	for _, v := range stages {
		if v < 0 {
			return true
		}
	}
	return false
}

// statusByTableIndex 扩展疲惫(7)。
func statusByTableIndexEx(st *battleStatus, idx int) bool {
	if idx == 7 {
		return st != nil && st.Tired
	}
	return statusByTableIndex(st, idx)
}

// sideEffectFirstStrikeFlat SideEffect 405：先出手附加固定伤害。
func sideEffectFirstStrikeFlat(d *tableloader.SkillDef, wentFirst bool) uint32 {
	if d == nil || !wentFirst || !skillHasSideEffect(d, 405) {
		return 0
	}
	args := sideEffectArgsFor(d, 405)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	if len(args) >= 1 && args[0] > 0 {
		return uint32(args[0])
	}
	return 0
}

// sideEffectBoostFlat SideEffect 413：对手有能力强化则附加固伤。
func sideEffectBoostFlat(d *tableloader.SkillDef, foeStages *[5]int8) uint32 {
	if d == nil || !skillHasSideEffect(d, 413) || !hasPositiveStages(foeStages) {
		return 0
	}
	args := sideEffectArgsFor(d, 413)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	if len(args) >= 1 && args[0] > 0 {
		return uint32(args[0])
	}
	return 0
}

// sideEffectDropFlat SideEffect 167：对手有能力下降则附加固伤。
func sideEffectDropFlat(d *tableloader.SkillDef, foeStages *[5]int8) uint32 {
	if d == nil || !skillHasSideEffect(d, 167) || !hasNegativeStages(foeStages) {
		return 0
	}
	args := sideEffectArgsFor(d, 167)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	if len(args) >= 1 && args[0] > 0 {
		return uint32(args[0])
	}
	return 0
}

// sideEffectStatusIndexFlat SideEffect 467：对手特定异常附加固伤。
func sideEffectStatusIndexFlat(d *tableloader.SkillDef, foe *battleStatus) uint32 {
	if d == nil || foe == nil || !skillHasSideEffect(d, 467) {
		return 0
	}
	args := sideEffectArgsFor(d, 467)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	idx, flat := 2, 50
	if len(args) >= 1 {
		idx = args[0]
	}
	if len(args) >= 2 && args[1] > 0 {
		flat = args[1]
	}
	if !statusByTableIndexEx(foe, idx) {
		return 0
	}
	return uint32(flat)
}

// sideEffectLowHPExecute SideEffect 456：对手体力不足阈值则秒杀（返回全额当前体力）。
func sideEffectLowHPExecute(d *tableloader.SkillDef, foeHP uint32) uint32 {
	if d == nil || foeHP == 0 || !skillHasSideEffect(d, 456) {
		return 0
	}
	args := sideEffectArgsFor(d, 456)
	thr := 250
	if len(args) >= 1 && args[0] > 0 {
		thr = args[0]
	}
	if int(foeHP) < thr {
		return foeHP
	}
	return 0
}

// sideEffectMinDamage SideEffect 135：伤害不低于 n。
func sideEffectMinDamage(d *tableloader.SkillDef, dmg uint32) uint32 {
	if d == nil || !skillHasSideEffect(d, 135) {
		return dmg
	}
	args := sideEffectArgsFor(d, 135)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	min := 0
	if len(args) >= 1 && args[0] > 0 {
		min = args[0]
	}
	if min > 0 && int(dmg) < min {
		return uint32(min)
	}
	return dmg
}

// applyMoreDamageSideEffects SideEffect 405/413/167/467/456/135/431。
func applyMoreDamageSideEffects(d *tableloader.SkillDef, dmg, foeHP uint32, foe *battleStatus, foeStages *[5]int8, wentFirst bool) uint32 {
	if d == nil {
		return dmg
	}
	if dmg > 0 {
		dmg = foeDropDamageMul(d, foeStages, dmg)
	}
	dmg += sideEffectFirstStrikeFlat(d, wentFirst)
	dmg += sideEffectBoostFlat(d, foeStages)
	dmg += sideEffectDropFlat(d, foeStages)
	dmg += sideEffectStatusIndexFlat(d, foe)
	if exec := sideEffectLowHPExecute(d, foeHP); exec > 0 {
		dmg = exec
	}
	dmg = sideEffectMinDamage(d, dmg)
	return dmg
}

func (st *BattleState) stagedDefIgnoreBoost(player bool) int {
	base, stage := st.EnemyDef, st.EnemyStages[stageDef]
	if player {
		base, stage = st.PlayerDef, st.PlayerStages[stageDef]
	}
	if stage > 0 {
		stage = 0
	}
	return int(float64(base) * stageMultiplier(int(stage)))
}

func (st *BattleState) stagedSpDefIgnoreBoost(player bool) int {
	base, stage := st.EnemySpDef, st.EnemyStages[stageSD]
	if player {
		base, stage = st.PlayerSpDef, st.PlayerStages[stageSD]
	}
	if stage > 0 {
		stage = 0
	}
	return int(float64(base) * stageMultiplier(int(stage)))
}

// defStatsForSkill 195：无视对手防御/特防正向能力。
func defStatsForSkill(st *BattleState, d *tableloader.SkillDef, foeIsPlayer bool) (def, spDef int) {
	if st == nil {
		return 1, 1
	}
	if d != nil && skillHasSideEffect(d, 195) {
		return st.stagedDefIgnoreBoost(foeIsPlayer), st.stagedSpDefIgnoreBoost(foeIsPlayer)
	}
	return st.stagedDef(foeIsPlayer), st.stagedSpDef(foeIsPlayer)
}

// maleDamageMulFromBuff SideEffect 98：对雄性伤害倍率。
func maleDamageMulFromBuff(atkBuff *battleBuff, foeGender int, dmg uint32) uint32 {
	if atkBuff == nil || dmg == 0 || atkBuff.MaleDmgRounds == 0 || foeGender != 1 {
		return dmg
	}
	mul := int(atkBuff.MaleDmgMul)
	if mul < 1 {
		mul = 2
	}
	return dmg * uint32(mul)
}

// applyCondStatusSelfBoost SideEffect 182：对手特定异常时概率自身能力上升。
func applyCondStatusSelfBoost(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	statusIdx, stat, chance, delta := 5, stageSpd, 100, 1
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
		statusIdx = slice[0]
	}
	if len(slice) >= 2 {
		stat = slice[1]
	}
	if len(slice) >= 3 {
		chance = slice[2]
	}
	if len(slice) >= 4 {
		delta = slice[3]
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 182) {
		return argOff
	}
	if chance > 100 {
		chance = 100
	}
	foe := pickFoeStatus(st, playerIsAtk)
	if !statusByTableIndexEx(foe, statusIdx) || stat < 0 || stat > stageSpd || rand.Intn(100) >= chance {
		return argOff
	}
	stages := pickSelfStages(st, playerIsAtk)
	stages[stat] = int8(clampStage(int(stages[stat]) + delta))
	return argOff
}

// applyBoostFoeStageChange SideEffect 418/437：对手有强化时改其能力。
func applyBoostFoeStageChange(st *BattleState, d *tableloader.SkillDef, eid int, playerIsAtk bool, args []int, argOff int) int {
	stat, delta := 0, -1
	if argOff < len(args) {
		stat = args[argOff]
		argOff++
	}
	if argOff < len(args) {
		delta = args[argOff]
		argOff++
	}
	if st == nil || d == nil || !skillHasSideEffect(d, eid) {
		return argOff
	}
	foeStages := pickFoeStages(st, playerIsAtk)
	if !hasPositiveStages(foeStages) {
		return argOff
	}
	if stageDropImmuneFromBuff(pickFoeBuff(st, playerIsAtk)) && delta < 0 {
		return argOff
	}
	if stat < 0 || stat > stageSpd {
		return argOff
	}
	foeStages[stat] = int8(clampStage(int(foeStages[stat]) + delta))
	return argOff
}

// applyCondStatusDrain SideEffect 472：对手特定异常时，造成伤害全部回血。
func applyCondStatusDrain(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, lost uint32) {
	if st == nil || d == nil || lost == 0 || !skillHasSideEffect(d, 472) {
		return
	}
	args := sideEffectArgsFor(d, 472)
	if len(args) == 0 {
		args = parseSideEffectArgs(d.SideEffectArg)
	}
	idx := 2
	if len(args) >= 1 {
		idx = args[0]
	}
	if !statusByTableIndexEx(pickFoeStatus(st, playerIsAtk), idx) {
		return
	}
	if playerIsAtk {
		applyHealCap(&st.PlayerHP, &st.PlayerMaxHP, lost)
	} else {
		applyHealCap(&st.EnemyHP, &st.EnemyMaxHP, lost)
	}
}

// applyFirstStrikeStage SideEffect 122：先手概率改敌能力。
func applyFirstStrikeStage(st *BattleState, d *tableloader.SkillDef, playerIsAtk, wentFirst bool, args []int, argOff int) int {
	stat, chance, delta := 0, 50, -1
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
	if st == nil || d == nil || !wentFirst || !skillHasSideEffect(d, 122) {
		return argOff
	}
	if chance > 100 {
		chance = 100
	}
	if stat < 0 || stat > stageSpd || rand.Intn(100) >= chance {
		return argOff
	}
	if stageDropImmuneFromBuff(pickFoeBuff(st, playerIsAtk)) && delta < 0 {
		return argOff
	}
	stages := pickFoeStages(st, playerIsAtk)
	stages[stat] = int8(clampStage(int(stages[stat]) + delta))
	return argOff
}

// applyRisingStatusChance SideEffect 181：概率上异常，连续同技提高几率。
func applyRisingStatusChance(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, consec int, args []int, argOff int) int {
	chance, idx, step, maxC := 15, 0, 5, 20
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
	if len(slice) >= 2 {
		idx = slice[1]
	}
	if len(slice) >= 3 {
		step = slice[2]
	}
	if len(slice) >= 4 {
		maxC = slice[3]
	}
	if st == nil || d == nil || !skillHasSideEffect(d, 181) {
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
	setStatusByTableIndex(foe, idx)
	return argOff
}

// applyOnHurtStageBoost SideEffect 123：受击时自身能力上升。
func applyOnHurtStageBoost(defBuff *battleBuff, selfStages *[5]int8) {
	if defBuff == nil || selfStages == nil || defBuff.OnHurtBoostRounds == 0 {
		return
	}
	stat := int(defBuff.OnHurtBoostStat)
	delta := int(defBuff.OnHurtBoostDelta)
	if delta == 0 {
		delta = 1
	}
	if stat < 0 || stat > stageSpd {
		return
	}
	selfStages[stat] = int8(clampStage(int(selfStages[stat]) + delta))
}
