package gameserver

import (
	"math/rand"
	"strconv"
	"strings"

	"niaohao/server/internal/tableloader"
)

// SideEffect MVP+（对齐本前端 Effect_*.as / SkillXML，不搬全量 effects.go）：
// 1 吸取；2 敌方半血以下威力翻倍；3 清自降；4/5 能力变化；6 反伤；7 同生共死；
// 8 手下留情；9 连续同技威力递增；10/11/12/14/15/16 异常；13 寄生种子；
// 17 蓄力；20/22 疲惫；21 反弹；28 削当前体力比例；29 固伤；30 后手威力翻倍；31 多段；
// 32 若干回合暴击+1/16；33 清敌强化；34 反击；35 惩罚；36 秒杀；37 残血威力；38 削最大体力；
// 39 削 PP；40 先手威力翻倍；41 火伤减半；42 电系翻倍；43 回血；44/50 物特减半；
// 45/51 防攻等同；46 护盾；47/48 免疫降能力/异常；49 下次减伤；52 技能失效；
// 53/54/90 攻伤倍率；55/56 属性交换/复制；57 回合回血；58 必中要害；59/71 献祭；
// 60/76 DoT；61/70 随机威力；62 延迟秒杀；63 转移自降；64 异常增伤；65 指定系；
// 66/67 击杀遗留；68 硬撑；72 未命中自灭；73 先手反伤；74/75 三选一异常；
// 77 固定回血；78/86 物/特必 miss；79 半血强化；80 半血等伤；81 必中；
// 84/92 受击异常；85 偷强化；87 回满 PP；88 概率倍伤；89 吸血；93 概率固伤；
// 94/99 石化/混乱；95 睡敌暴击；96/97 烧冻翻倍；100 残血增伤。

const (
	stageAtk = 0
	stageDef = 1
	stageSA  = 2
	stageSD  = 3
	stageSpd = 4
)

type battleStatus struct {
	Para       bool
	Poison     bool
	Burn       bool
	Freeze     bool
	Fear       bool
	Tired      bool // 疲惫：跳过 1 回合
	Sleep      bool
	Flammable  bool // 易燃：火系伤害×1.5
}

func parseSideEffectIDs(s string) []int {
	parts := strings.Fields(s)
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		id, err := strconv.Atoi(p)
		if err != nil || id <= 0 {
			continue
		}
		out = append(out, id)
	}
	return out
}

func parseSideEffectArgs(s string) []int {
	parts := strings.Fields(s)
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	return out
}

func skillHasSideEffect(d *tableloader.SkillDef, eid int) bool {
	if d == nil || d.SideEffect == "" {
		return false
	}
	for _, id := range parseSideEffectIDs(d.SideEffect) {
		if id == eid {
			return true
		}
	}
	return false
}

func stageMultiplier(stage int) float64 {
	if stage > 6 {
		stage = 6
	}
	if stage < -6 {
		stage = -6
	}
	if stage >= 0 {
		return float64(2+stage) / 2
	}
	return 2 / float64(2-stage)
}

func clampStage(v int) int {
	if v > 6 {
		return 6
	}
	if v < -6 {
		return -6
	}
	return v
}

func countEffect(ids []int, eid int) int {
	n := 0
	for _, id := range ids {
		if id == eid {
			n++
		}
	}
	return n
}

func applyStatGroups(stages *[5]int8, args []int, maxGroups int) int {
	return applyStatGroupsImmune(stages, args, maxGroups, false)
}

func applyStatGroupsImmune(stages *[5]int8, args []int, maxGroups int, dropImmune bool) int {
	if stages == nil || maxGroups <= 0 {
		return 0
	}
	consumed := 0
	n := 0
	for i := 0; i+2 < len(args) && n < maxGroups; i += 3 {
		stat, chance, delta := args[i], args[i+1], args[i+2]
		consumed = i + 3
		n++
		if chance < 100 && rand.Intn(100) >= chance {
			continue
		}
		if stat < 0 || stat > stageSpd {
			continue
		}
		if dropImmune && delta < 0 {
			continue
		}
		stages[stat] = int8(clampStage(int(stages[stat]) + delta))
	}
	return consumed
}

func clearNegativeStages(stages *[5]int8) {
	if stages == nil {
		return
	}
	for i := range stages {
		if stages[i] < 0 {
			stages[i] = 0
		}
	}
}

func clearPositiveStages(stages *[5]int8) {
	if stages == nil {
		return
	}
	for i := range stages {
		if stages[i] > 0 {
			stages[i] = 0
		}
	}
}

func setStatus(st *battleStatus, eid int) {
	if st == nil {
		return
	}
	switch eid {
	case 10:
		st.Para = true
	case 11:
		st.Poison = true
	case 12:
		st.Burn = true
	case 14:
		st.Freeze = true
	case 15:
		st.Fear = true
	case 16:
		st.Sleep = true
	case 20, 22:
		st.Tired = true
	}
}

// sideEffectHitCount SideEffect 31：1 回合 m~n 次；无则 1。
func sideEffectHitCount(d *tableloader.SkillDef) int {
	if d == nil || !skillHasSideEffect(d, 31) {
		return 1
	}
	args := parseSideEffectArgs(d.SideEffectArg)
	lo, hi := 2, 3
	if len(args) >= 1 && args[0] > 0 {
		lo = args[0]
	}
	if len(args) >= 2 && args[1] > 0 {
		hi = args[1]
	}
	if hi < lo {
		hi = lo
	}
	if lo < 1 {
		lo = 1
	}
	if hi > 8 {
		hi = 8
	}
	if hi == lo {
		return lo
	}
	return lo + rand.Intn(hi-lo+1)
}

// sideEffectFixedDamage SideEffect 29：额外固定伤害（粉伤，勿并入 lostHP）。
func sideEffectFixedDamage(d *tableloader.SkillDef) uint32 {
	if d == nil || !skillHasSideEffect(d, 29) {
		return 0
	}
	args := parseSideEffectArgs(d.SideEffectArg)
	if len(args) < 1 || args[0] <= 0 {
		return 0
	}
	return uint32(args[0])
}

// sideEffectChanceFixedDamage SideEffect 93：n% 概率额外 m 点固伤（粉伤）。
func sideEffectChanceFixedDamage(d *tableloader.SkillDef) uint32 {
	if d == nil || !skillHasSideEffect(d, 93) {
		return 0
	}
	args := parseSideEffectArgs(d.SideEffectArg)
	if len(args) < 2 || args[1] <= 0 {
		return 0
	}
	chance := args[0]
	if chance > 100 {
		chance = 100
	}
	if chance < 100 && rand.Intn(100) >= chance {
		return 0
	}
	return uint32(args[1])
}

// sideEffectPinkDamage SideEffect 28/29/93 粉伤合计：只扣 HP，不进 lostHP（客户端用 remainHP 差显示粉色）。
func sideEffectPinkDamage(d *tableloader.SkillDef, foeHP uint32) uint32 {
	return sideEffectFixedDamage(d) + sideEffectChanceFixedDamage(d) + sideEffectPercentHPDamage(d, foeHP)
}

// applyPinkDamage 在公式伤之后额外扣血，不返回给 lostHP。
func applyPinkDamage(hp *uint32, pink uint32) {
	if pink == 0 || hp == nil {
		return
	}
	_ = applyDamage(hp, pink)
}

// skillPowerAdj 威力修正上下文（2/9/30/35/37/40/113/1901）。
// FoeMaxHP：受击方最大体力（效果 2 半血判定）。
type skillPowerAdj struct {
	FoeHP, FoeMaxHP   uint32
	SelfHP, SelfMaxHP uint32
	GoingFirst        bool
	ConsecCount       uint32
	FoeStages         *[5]int8
	SelfDV            int
	FoeAliveExtra     int // 2237：对方相对己方多存活的精灵数
}

// adjustSkillPower 按 SideEffect 调整技能威力。
func adjustSkillPower(d *tableloader.SkillDef, base int, adj skillPowerAdj) int {
	if d == nil || base <= 0 {
		return base
	}
	power := base
	if skillHasSideEffect(d, 2) && adj.FoeMaxHP > 0 && adj.FoeHP*2 < adj.FoeMaxHP {
		power *= 2
	}
	if skillHasSideEffect(d, 30) && !adj.GoingFirst {
		power *= 2
	}
	if skillHasSideEffect(d, 40) && adj.GoingFirst {
		power *= 2
	}
	if skillHasSideEffect(d, 9) {
		args := parseSideEffectArgs(d.SideEffectArg)
		inc, cap := 20, 80
		if len(args) >= 1 && args[0] > 0 {
			inc = args[0]
		}
		if len(args) >= 2 && args[1] > 0 {
			cap = args[1]
		}
		bonus := int(adj.ConsecCount) * inc
		if bonus > cap {
			bonus = cap
		}
		power += bonus
	}
	if skillHasSideEffect(d, 35) && adj.FoeStages != nil {
		sum := 0
		for _, stg := range adj.FoeStages {
			if stg > 0 {
				sum += int(stg)
			}
		}
		power += 20 * sum
	}
	power = adjustSkillPowerSelfHP(d, power, adj.SelfHP, adj.SelfMaxHP)
	power = dvSkillPower(d, power, adj.SelfDV)
	power = dvPower1470(d, power, adj.SelfDV)
	if bonus := powerBonus2237(d, adj.FoeAliveExtra); bonus > 0 {
		power += bonus
	}
	if power < 1 {
		power = 1
	}
	return power
}

// sameLifeDamage SideEffect 7：敌方体力高于自身时才能命中，伤害=体力差（减到与自己相同）。
// ok=true 表示本技能走该逻辑（命中条件不满足时 dmg=0）。
func sameLifeDamage(d *tableloader.SkillDef, selfHP, foeHP uint32) (dmg uint32, ok bool) {
	if d == nil || !skillHasSideEffect(d, 7) {
		return 0, false
	}
	if foeHP <= selfHP {
		return 0, true
	}
	return foeHP - selfHP, true
}

func advanceConsecutiveSkill(id *uint32, count *uint32, skillID uint32) {
	if id == nil || count == nil {
		return
	}
	if skillID == 0 {
		*id, *count = 0, 0
		return
	}
	if *id == skillID {
		*count++
	} else {
		*id = skillID
		*count = 0
	}
}

func tickCritBonusRounds(st *BattleState) {
	if st == nil {
		return
	}
	if st.PlayerCritBonusRounds > 0 {
		st.PlayerCritBonusRounds--
	}
	if st.EnemyCritBonusRounds > 0 {
		st.EnemyCritBonusRounds--
	}
}

func critExtraWithRounds(baseExtra int, rounds byte) int {
	if rounds > 0 {
		return baseExtra + 1
	}
	return baseExtra
}

// applyLeaveOneHP SideEffect 8/112：致死时余 1。
func applyLeaveOneHP(d *tableloader.SkillDef, hpBefore, dmg uint32) uint32 {
	if d == nil || dmg == 0 || hpBefore <= 1 {
		return dmg
	}
	if !skillHasSideEffect(d, 8) && !skillHasSideEffect(d, 112) {
		return dmg
	}
	if dmg >= hpBefore {
		return hpBefore - 1
	}
	return dmg
}

// applySkillSideEffects 在造成 lost 伤害后结算。playerIsAtk=true 表示玩家出招。
// wentFirst：本技能是否先手（148/147/186/402 等后手效果依赖）。
func (s *Server) applySkillSideEffects(st *BattleState, skillID uint32, lost uint32, playerIsAtk, wentFirst bool) {
	if st == nil || skillID == 0 {
		return
	}
	d := s.skillDef(int(skillID))
	if d == nil || d.SideEffect == "" {
		return
	}
	ids := parseSideEffectIDs(d.SideEffect)
	args := parseSideEffectArgs(d.SideEffectArg)
	argOff := 0
	done4, done5 := false, false

	for _, eid := range ids {
		switch eid {
		case 1:
			if lost == 0 {
				continue
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
		case 101:
			heal := sideEffectDrainPercent(d, lost)
			if playerIsAtk {
				applyHealCap(&st.PlayerHP, &st.PlayerMaxHP, heal)
			} else {
				applyHealCap(&st.EnemyHP, &st.EnemyMaxHP, heal)
			}
		case 105:
			heal := sideEffectDrainDenom(d, lost)
			if playerIsAtk {
				applyHealCap(&st.PlayerHP, &st.PlayerMaxHP, heal)
			} else {
				applyHealCap(&st.EnemyHP, &st.EnemyMaxHP, heal)
			}
		case 154:
			foe := pickFoeStatus(st, playerIsAtk)
			heal := sideEffectCondDrain(d, foe, lost)
			if playerIsAtk {
				applyHealCap(&st.PlayerHP, &st.PlayerMaxHP, heal)
			} else {
				applyHealCap(&st.EnemyHP, &st.EnemyMaxHP, heal)
			}
		case 3:
			clearNegativeStages(pickSelfStages(st, playerIsAtk))
		case 4:
			if done4 {
				continue
			}
			done4 = true
			take := countEffect(ids, 4)
			if take < 1 {
				take = 1
			}
			used := applyStatGroups(pickSelfStages(st, playerIsAtk), args[argOff:], take)
			argOff += used
		case 5:
			if done5 {
				continue
			}
			done5 = true
			take := countEffect(ids, 5)
			if take < 1 {
				take = 1
			}
			used := applyStatGroupsImmune(pickFoeStages(st, playerIsAtk), args[argOff:], take,
				stageDropImmuneFromBuff(pickFoeBuff(st, playerIsAtk)))
			argOff += used
			if playerIsAtk {
				clampEnemyNegativeStages(st)
			}
		case 6:
			// 反伤：自身承受 lost/n
			if lost == 0 {
				continue
			}
			n := 2
			if argOff < len(args) && args[argOff] > 0 {
				n = args[argOff]
				argOff++
			} else if len(args) > 0 && args[0] > 0 {
				n = args[0]
			}
			if n < 1 {
				n = 1
			}
			recoil := lost / uint32(n)
			if recoil < 1 {
				recoil = 1
			}
			if playerIsAtk {
				_ = applyDamage(&st.PlayerHP, recoil)
			} else {
				_ = applyDamage(&st.EnemyHP, recoil)
			}
		case 10, 11, 12, 14, 15, 16:
			chance := 10
			if argOff < len(args) {
				chance = args[argOff]
				argOff++
			} else if len(args) > 0 {
				chance = args[0]
			}
			if chance < 100 && rand.Intn(100) >= chance {
				continue
			}
			if playerIsAtk {
				if statusImmuneFromBuff(&st.EnemyBuff) || !canApplyEnemyBattleStatus(st, eid) {
					continue
				}
				setStatus(&st.EnemyStatus, eid)
			} else {
				if statusImmuneFromBuff(&st.PlayerBuff) {
					continue
				}
				setStatus(&st.PlayerStatus, eid)
			}
		case 20:
			chance, rounds := 100, 1
			if argOff < len(args) {
				chance = args[argOff]
				argOff++
			}
			if argOff < len(args) {
				rounds = args[argOff]
				argOff++
			}
			_ = rounds
			if chance < 100 && rand.Intn(100) >= chance {
				continue
			}
			setStatus(pickSelfStatus(st, playerIsAtk), 20)
		case 22:
			chance, rounds := 10, 1
			if argOff < len(args) {
				chance = args[argOff]
				argOff++
			}
			if argOff < len(args) && args[argOff] > 0 {
				rounds = args[argOff]
				argOff++
			}
			_ = rounds
			if chance < 100 && rand.Intn(100) >= chance {
				continue
			}
			if playerIsAtk {
				if statusImmuneFromBuff(&st.EnemyBuff) || !canApplyEnemyBattleStatus(st, 22) {
					continue
				}
			} else if statusImmuneFromBuff(&st.PlayerBuff) {
				continue
			}
			setStatus(pickFoeStatus(st, playerIsAtk), 22)
		case 33:
			clearPositiveStages(pickFoeStages(st, playerIsAtk))
		case 32:
			rounds := 3
			if argOff < len(args) && args[argOff] > 0 {
				rounds = args[argOff]
				argOff++
			} else if len(args) > 0 && args[0] > 0 {
				rounds = args[0]
			}
			if rounds < 1 {
				rounds = 1
			}
			if rounds > 16 {
				rounds = 16
			}
			if playerIsAtk {
				st.PlayerCritBonusRounds = byte(rounds)
			} else {
				st.EnemyCritBonusRounds = byte(rounds)
			}
		case 13, 21, 41, 42, 44, 45, 46, 47, 48, 49, 50, 51, 53, 54, 57, 58, 60, 65, 68, 76, 77, 78, 81, 84, 86, 89, 90, 91, 92, 98, 104, 106, 108, 109, 110, 116, 117, 123, 125, 126, 128, 156, 190, 433, 439, 441, 448, 463, 478, 545:
			argOff = applyOneOngoingBuff(st, eid, args, argOff, playerIsAtk)
		case 63:
			transferNegativeStages(pickSelfStages(st, playerIsAtk), pickFoeStages(st, playerIsAtk))
			if playerIsAtk {
				clampEnemyNegativeStages(st)
			}
		case 74, 75:
			sideEffectStatusTriple(d, st, playerIsAtk)
		case 38:
			if playerIsAtk {
				sideEffectCutMaxHP(d, &st.EnemyMaxHP, &st.EnemyHP)
			} else {
				sideEffectCutMaxHP(d, &st.PlayerMaxHP, &st.PlayerHP)
			}
		case 39:
			if playerIsAtk {
				sideEffectPPDrain(d, st.EnemySkills)
			} else {
				sideEffectPPDrain(d, st.PlayerSkills)
			}
		case 43:
			n := 2
			if argOff < len(args) && args[argOff] > 0 {
				n = args[argOff]
				argOff++
			} else if len(args) > 0 && args[0] > 0 {
				n = args[0]
			}
			if n < 1 {
				n = 1
			}
			if playerIsAtk {
				heal := st.PlayerMaxHP / uint32(n)
				if heal < 1 {
					heal = 1
				}
				st.PlayerHP += heal
				if st.PlayerHP > st.PlayerMaxHP {
					st.PlayerHP = st.PlayerMaxHP
				}
			} else {
				heal := st.EnemyMaxHP / uint32(n)
				if heal < 1 {
					heal = 1
				}
				st.EnemyHP += heal
				if st.EnemyHP > st.EnemyMaxHP {
					st.EnemyHP = st.EnemyMaxHP
				}
			}
		case 79:
			if playerIsAtk {
				_ = applyHalfHPStageBoost(&st.PlayerHP, st.PlayerMaxHP, &st.PlayerStages, nil, 0)
			} else {
				_ = applyHalfHPStageBoost(&st.EnemyHP, st.EnemyMaxHP, &st.EnemyStages, nil, 0)
			}
		case 85:
			stealPositiveStages(pickSelfStages(st, playerIsAtk), pickFoeStages(st, playerIsAtk))
			if playerIsAtk {
				clampEnemyNegativeStages(st)
			}
		case 87:
			if playerIsAtk {
				restoreAllSkillPP(st.PlayerSkills, func(id int) *tableloader.SkillDef { return s.skillDef(id) })
			} else {
				restoreAllSkillPP(st.EnemySkills, func(id int) *tableloader.SkillDef { return s.skillDef(id) })
			}
		case 94:
			chance := 10
			if argOff < len(args) && args[argOff] > 0 {
				chance = args[argOff]
				argOff++
			}
			if chance > 100 {
				chance = 100
			}
			if rand.Intn(100) < chance {
				foe := pickFoeStatus(st, playerIsAtk)
				if !statusImmuneFromBuff(pickFoeBuff(st, playerIsAtk)) {
					setStatus(foe, 15)
					mirrorSyncChanges(st, !playerIsAtk, func(targetIsPlayer bool) {
						setStatus(pickSelfStatus(st, targetIsPlayer), 15)
					})
				}
			}
		case 99:
			chance := 10
			if argOff < len(args) && args[argOff] > 0 {
				chance = args[argOff]
				argOff++
			}
			if chance > 100 {
				chance = 100
			}
			if rand.Intn(100) < chance {
				foe := pickFoeStatus(st, playerIsAtk)
				if !statusImmuneFromBuff(pickFoeBuff(st, playerIsAtk)) {
					setStatus(foe, 15)
				}
			}
		case 55, 56:
			argOff = applyTypeSwapOrCopy(st, d, playerIsAtk, args, argOff)
		case 148:
			argOff = applySecondStrikeStage(st, d, 148, playerIsAtk, wentFirst, args, argOff)
		case 186:
			argOff = applySecondStrikeStage(st, d, 186, playerIsAtk, wentFirst, args, argOff)
		case 147:
			argOff = applySecondStrikeStatus(st, d, playerIsAtk, wentFirst, args, argOff)
		case 122:
			argOff = applyFirstStrikeStage(st, d, playerIsAtk, wentFirst, args, argOff)
		case 182:
			argOff = applyCondStatusSelfBoost(st, d, playerIsAtk, args, argOff)
		case 418:
			argOff = applyBoostFoeStageChange(st, d, 418, playerIsAtk, args, argOff)
		case 437:
			argOff = applyBoostFoeStageChange(st, d, 437, playerIsAtk, args, argOff)
		case 181:
			consec := 1
			if playerIsAtk {
				consec = int(st.PlayerConsecSkillCount)
			} else {
				consec = int(st.EnemyConsecSkillCount)
			}
			argOff = applyRisingStatusChance(st, d, playerIsAtk, consec, args, argOff)
		case 472:
			applyCondStatusDrain(st, d, playerIsAtk, lost)
		case 172:
			applySecondStrikeDrain(st, d, playerIsAtk, wentFirst, lost)
		case 178:
			selfType := st.EnemyType
			if playerIsAtk {
				selfType = st.PlayerType
			}
			applySameTypeDrain(st, d, playerIsAtk, lost, selfType)
		case 194:
			applyCondStatusDrainEx(st, d, playerIsAtk, lost)
		case 173:
			argOff = applyFirstStrikeStatus(st, d, playerIsAtk, wentFirst, args, argOff)
		case 474:
			argOff = applyFirstStrikeSelfStage(st, d, playerIsAtk, wentFirst, args, argOff)
		case 430:
			argOff = applyClearBoostSelfStage(st, d, playerIsAtk, args, argOff)
		case 453:
			argOff = applyClearBoostFoeStatus(st, d, playerIsAtk, args, argOff)
		case 434:
			argOff = applySelfBoostStatus(st, d, playerIsAtk, args, argOff)
		case 438, 410:
			argOff = applyChanceMaxHPHeal(st, d, playerIsAtk, args, argOff)
		case 175:
			argOff = applyCondStatusSelfStage(st, d, playerIsAtk, args, argOff)
		case 184:
			argOff = applyCondBoostSelfStage(st, d, playerIsAtk, args, argOff)
		case 464:
			skType, foeType := 0, st.EnemyType
			if d != nil {
				skType = d.Type
			}
			if !playerIsAtk {
				foeType = st.PlayerType
			}
			applyTypeAdvantageBurn(st, d, playerIsAtk, skType, foeType)
		case 119:
			applyOddEvenHitEffect(st, d, playerIsAtk, lost)
		case 121:
			selfType := st.EnemyType
			if playerIsAtk {
				selfType = st.PlayerType
			}
			argOff = applySameTypePara(st, d, playerIsAtk, selfType, args, argOff)
		case 124:
			argOff = applyRandomStageDrop(st, d, playerIsAtk, args, argOff)
		case 145:
			applyPoisonMaxHPHeal(st, d, playerIsAtk)
		case 151:
			argOff = applyBurnCondTired(st, d, playerIsAtk, args, argOff)
		case 159:
			argOff = applyLowHPStatus(st, d, playerIsAtk, args, argOff)
		case 415:
			applyHighDamageHeal(st, d, playerIsAtk, lost)
		case 451:
			argOff = applyRandomDotStatus(st, d, playerIsAtk, args, argOff)
		case 103:
			argOff = applyChanceWeakness(st, d, playerIsAtk, args, argOff)
		case 114:
			argOff = applyChanceFlammable(st, d, playerIsAtk, args, argOff)
		case 120:
			argOff = applyCoinFlipHPCut(st, d, playerIsAtk, args, argOff)
		case 143:
			applyInvertFoeBoosts(st, d, playerIsAtk)
		case 196:
			argOff = applyDualStageChange196(st, d, playerIsAtk, wentFirst, args, argOff)
		case 201:
			argOff = applyPartyOrSelfHeal201(st, d, playerIsAtk, args, argOff)
		case 202:
			applyNextEnterAtkDef202(st, d, playerIsAtk)
		case 444:
			applyPPSwap444(st, d, playerIsAtk, func(id int) *tableloader.SkillDef { return s.skillDef(id) })
		case 445:
			applyRewardCoins445(st, d)
		case 449:
			argOff = applyDropCondStatus449(st, d, playerIsAtk, args, argOff)
		case 450:
			argOff = applyRandomHealRange450(st, d, playerIsAtk, args, argOff)
		case 454:
			argOff = applyLowHPPriority454(st, d, playerIsAtk, args, argOff)
		case 458:
			applyFirstStrikeDrain458(st, d, playerIsAtk, wentFirst, lost)
		case 460:
			argOff = applyFearWithBoostBonus460(st, d, playerIsAtk, args, argOff)
		case 461:
			argOff = applyLowHPMustCrit461(st, d, playerIsAtk, args, argOff)
		case 465:
			consec := 1
			if playerIsAtk {
				consec = int(st.PlayerConsecSkillCount)
			} else {
				consec = int(st.EnemyConsecSkillCount)
			}
			argOff = applyRisingTired465(st, d, playerIsAtk, consec, args, argOff)
		case 466:
			argOff = applyFlatHeal466(st, d, playerIsAtk, args, argOff)
		case 471:
			argOff = applyFirstStrikeStatusImmune471(st, d, playerIsAtk, wentFirst, args, argOff)
		case 475:
			argOff = applyLowDamageMustCrit475(st, d, playerIsAtk, lost, args, argOff)
		case 476:
			argOff = applySecondStrikeFlatHeal476(st, d, playerIsAtk, wentFirst, args, argOff)
		case 482:
			argOff = applyChancePriority482(st, d, playerIsAtk, args, argOff)
		case 508:
			argOff = applyFlatReduceNext508(st, d, playerIsAtk, args, argOff)
		case 687:
			argOff = applyStatusDrain687(st, d, playerIsAtk, lost, args, argOff)
		case 1635:
			argOff = applyHealAndDelayFull1635(st, d, playerIsAtk, args, argOff)
		case 485:
			applyClearBoostFullHeal485(st, d, playerIsAtk)
		case 487:
			argOff = applyHighHPAtkBoost487(st, d, playerIsAtk, args, argOff)
		case 494:
			applyClearFoeBoost494(st, d, playerIsAtk)
		case 495:
			argOff = applyStatusExecute495(st, d, playerIsAtk, args, argOff)
		case 691:
			argOff = applyChanceOHKO691(st, d, playerIsAtk, args, argOff)
		case 700:
			argOff = applyFirstStrikePPDrain700(st, d, playerIsAtk, wentFirst, args, argOff)
		case 773:
			applyLowHPSwap773(st, d, playerIsAtk)
		case 935:
			argOff = applyHighHPStatus935(st, d, playerIsAtk, args, argOff)
		case 976:
			argOff = applyDispelWithAttrBlock976(st, d, playerIsAtk, args, argOff)
		case 1083:
			applySecondStrikeDispel1083(st, d, playerIsAtk, wentFirst)
		case 1211:
			argOff = applyClearAllStagesAbsorb1211(st, d, playerIsAtk, args, argOff)
		case 1248:
			argOff = applyAbnormalExtraStatus1248(st, d, playerIsAtk, args, argOff)
		case 1257:
			argOff = applyNoStatusDrain1257(st, d, playerIsAtk, args, argOff)
		case 1603:
			argOff = applyChancePPDrain1603(st, d, playerIsAtk, args, argOff)
		case 1605:
			argOff = applyChanceStatus1605(st, d, playerIsAtk, args, argOff)
		case 1850:
			argOff = applyDispelBothBoost1850(st, d, playerIsAtk, args, argOff)
		case 1925:
			argOff = applyMaxHPDrain1925(st, d, playerIsAtk, args, argOff)
		case 2236:
			argOff = applySupportDouble2236(st, d, playerIsAtk, args, argOff)
		case 134:
			applyLowDamagePPBoost(st, d, playerIsAtk, lost, func(id int) *tableloader.SkillDef { return s.skillDef(id) })
		case 180:
			clearFoeOngoingBuffs(st, playerIsAtk)
		case 107:
			applyLowDamageSelfBoost(st, d, playerIsAtk, lost)
		case 473:
			applyLowDamageSelfBoostEx(st, d, playerIsAtk, lost)
		case 83:
			gid := 0
			if s.cfg.Catalog != nil {
				if playerIsAtk {
					gid = s.cfg.Catalog.PetGender(st.PlayerPetID)
				} else {
					gid = s.cfg.Catalog.PetGender(st.EnemyID)
				}
			}
			applyGenderSelfBuff(st, d, playerIsAtk, gid)
		case 29, 31, 8, 2, 7, 9, 28, 30, 35, 36, 37, 40, 61, 64, 70, 72, 80, 82, 88, 93, 95, 96, 97, 100, 102, 111, 112, 113, 115, 118, 129, 130, 131, 132, 133, 135, 139, 141, 162, 167, 168, 179, 188, 192, 193, 195, 401, 402, 405, 411, 413, 421, 422, 428, 429, 431, 436, 447, 455, 456, 459, 467, 468, 484, 488, 795, 1470, 1901, 2237:
			// 伤害/威力/命中/暴击阶段已处理
		case 17, 34, 52, 59, 62, 66, 67, 71, 73, 158:
			// 蓄力/反击/献祭/击杀遗留等：见 extra + 回合接线（158 在击杀后补）
		}
	}
	// 命中后触发进攻方 buff（104/109/116/117/441）
	if lost > 0 {
		applyOnHitAttackerBuffs(st, d, playerIsAtk, wentFirst, lost)
	}
}

func pickSelfStages(st *BattleState, playerIsAtk bool) *[5]int8 {
	if playerIsAtk {
		return &st.PlayerStages
	}
	return &st.EnemyStages
}

func pickFoeStages(st *BattleState, playerIsAtk bool) *[5]int8 {
	if playerIsAtk {
		return &st.EnemyStages
	}
	return &st.PlayerStages
}

func pickSelfStatus(st *BattleState, playerIsAtk bool) *battleStatus {
	if playerIsAtk {
		return &st.PlayerStatus
	}
	return &st.EnemyStatus
}

func pickFoeStatus(st *BattleState, playerIsAtk bool) *battleStatus {
	if playerIsAtk {
		return &st.EnemyStatus
	}
	return &st.PlayerStatus
}

func (st *BattleState) stagedAtk(player bool) int {
	if st == nil {
		return 1
	}
	if player && st.PlayerBuff.EqualFoeAtkRounds > 0 {
		return st.stagedAtkRaw(false)
	}
	if !player && st.EnemyBuff.EqualFoeAtkRounds > 0 {
		return st.stagedAtkRaw(true)
	}
	return st.stagedAtkRaw(player)
}

func (st *BattleState) stagedAtkRaw(player bool) int {
	base, stage := st.EnemyAtk, st.EnemyStages[stageAtk]
	buff := &st.EnemyBuff
	if player {
		base, stage = st.PlayerAtk, st.PlayerStages[stageAtk]
		buff = &st.PlayerBuff
	}
	stage = nullifyPositiveStage(buff, stage)
	return int(float64(base) * stageMultiplier(int(stage)))
}

func (st *BattleState) stagedDef(player bool) int {
	if st == nil {
		return 1
	}
	if player && st.PlayerBuff.EqualFoeDefRounds > 0 {
		return st.stagedDefRaw(false)
	}
	if !player && st.EnemyBuff.EqualFoeDefRounds > 0 {
		return st.stagedDefRaw(true)
	}
	return st.stagedDefRaw(player)
}

func (st *BattleState) stagedDefRaw(player bool) int {
	base, stage := st.EnemyDef, st.EnemyStages[stageDef]
	buff := &st.EnemyBuff
	if player {
		base, stage = st.PlayerDef, st.PlayerStages[stageDef]
		buff = &st.PlayerBuff
	}
	stage = nullifyPositiveStage(buff, stage)
	return int(float64(base) * stageMultiplier(int(stage)))
}

func (st *BattleState) stagedSpAtk(player bool) int {
	base, stage := st.EnemySpAtk, st.EnemyStages[stageSA]
	buff := &st.EnemyBuff
	if player {
		base, stage = st.PlayerSpAtk, st.PlayerStages[stageSA]
		buff = &st.PlayerBuff
	}
	stage = nullifyPositiveStage(buff, stage)
	return int(float64(base) * stageMultiplier(int(stage)))
}

func (st *BattleState) stagedSpDef(player bool) int {
	base, stage := st.EnemySpDef, st.EnemyStages[stageSD]
	buff := &st.EnemyBuff
	if player {
		base, stage = st.PlayerSpDef, st.PlayerStages[stageSD]
		buff = &st.PlayerBuff
	}
	stage = nullifyPositiveStage(buff, stage)
	return int(float64(base) * stageMultiplier(int(stage)))
}

func (st *BattleState) stagedSpd(player bool) int {
	base, stage := st.EnemySpd, st.EnemyStages[stageSpd]
	buff := &st.EnemyBuff
	if player {
		base, stage = st.PlayerSpd, st.PlayerStages[stageSpd]
		buff = &st.PlayerBuff
	}
	stage = nullifyPositiveStage(buff, stage)
	v := int(float64(base) * stageMultiplier(int(stage)))
	if player && st.PlayerStatus.Para {
		v /= 2
	}
	if !player && st.EnemyStatus.Para {
		v /= 2
	}
	return v
}

func nullifyPositiveStage(b *battleBuff, stage int8) int8 {
	if b != nil && b.BoostNullRounds > 0 && stage > 0 {
		return 0
	}
	return stage
}

func tickStatusDamage(st *BattleState) {
	if st == nil {
		return
	}
	dot := func(hp *uint32, max uint32, burn, poison bool) {
		if *hp == 0 || max == 0 || (!burn && !poison) {
			return
		}
		d := max / 8
		if d < 1 {
			d = 1
		}
		if *hp > d {
			*hp -= d
		} else {
			*hp = 0
		}
	}
	dot(&st.PlayerHP, st.PlayerMaxHP, st.PlayerStatus.Burn, st.PlayerStatus.Poison)
	dot(&st.EnemyHP, st.EnemyMaxHP, st.EnemyStatus.Burn, st.EnemyStatus.Poison)
}

func tickPlayerStatusDamage(st *BattleState) {
	if st == nil || st.PlayerHP == 0 || st.PlayerMaxHP == 0 {
		return
	}
	if !st.PlayerStatus.Burn && !st.PlayerStatus.Poison {
		return
	}
	d := st.PlayerMaxHP / 8
	if d < 1 {
		d = 1
	}
	if st.PlayerHP > d {
		st.PlayerHP -= d
	} else {
		st.PlayerHP = 0
	}
}

// consumeSkipStatus 回合开始结算异常跳过；睡/冻/怕/疲惫消耗后清除，麻痹 25%。
func consumeSkipStatus(st *battleStatus) bool {
	if st == nil {
		return false
	}
	if st.Sleep || st.Freeze || st.Fear || st.Tired {
		st.Sleep, st.Freeze, st.Fear, st.Tired = false, false, false, false
		return true
	}
	return st.Para && rand.Intn(4) == 0
}

func skipTurnFromPara(status battleStatus) bool {
	if status.Sleep || status.Freeze || status.Fear || status.Tired {
		return true
	}
	return status.Para && rand.Intn(4) == 0
}
