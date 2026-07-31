package gameserver

import (
	"niaohao/server/internal/tableloader"
)

// sideEffectCounterDamage SideEffect 34：将自身所受伤害的 n 倍反击（Power=0 反击技）。
// arg 为倍率：2=200%。无上次受伤则 0。
func sideEffectCounterDamage(d *tableloader.SkillDef, lastTaken uint32) uint32 {
	if d == nil || !skillHasSideEffect(d, 34) || lastTaken == 0 {
		return 0
	}
	args := parseSideEffectArgs(d.SideEffectArg)
	mul := 2
	if len(args) >= 1 && args[0] > 0 {
		// 表里常写 2 表示 200%；若写小数语义则前端用整数 2
		mul = args[0]
	}
	if mul < 1 {
		mul = 1
	}
	if mul > 8 {
		mul = 8
	}
	return lastTaken * uint32(mul)
}

// noteLastDamageTaken 记录本回合受到的实际伤害（供 34/73）。
func noteLastDamageTaken(st *BattleState, playerIsDef bool, lost uint32) {
	if st == nil || lost == 0 {
		return
	}
	if playerIsDef {
		st.PlayerLastTaken = lost
	} else {
		st.EnemyLastTaken = lost
	}
}

// tryInvalidateSkill SideEffect 52：先手且速度更快 → 令对方下 1 个技能失效。
func tryInvalidateSkill(st *BattleState, d *tableloader.SkillDef, playerIsAtk, wentFirst bool) {
	if st == nil || d == nil || !skillHasSideEffect(d, 52) || !wentFirst {
		return
	}
	selfSpd := st.stagedSpd(playerIsAtk)
	foeSpd := st.stagedSpd(!playerIsAtk)
	if selfSpd <= foeSpd {
		return
	}
	if playerIsAtk {
		st.EnemySkillFail = true
	} else {
		st.PlayerSkillFail = true
	}
}

// consumeSkillFail 若被 52 标记则消耗并返回 true（本技能失效）。
func consumeSkillFail(st *BattleState, playerIsAtk bool) bool {
	if st == nil {
		return false
	}
	if playerIsAtk {
		if st.PlayerSkillFail {
			st.PlayerSkillFail = false
			return true
		}
		return false
	}
	if st.EnemySkillFail {
		st.EnemySkillFail = false
		return true
	}
	return false
}

// armDoom SideEffect 62：n 回合后若自身存活则秒杀对方。
func armDoom(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool) {
	if st == nil || d == nil || !skillHasSideEffect(d, 62) {
		return
	}
	args := parseSideEffectArgs(d.SideEffectArg)
	n := 3
	if len(args) >= 1 && args[0] > 0 {
		n = args[0]
	}
	if n < 1 {
		n = 1
	}
	if n > 16 {
		n = 16
	}
	if playerIsAtk {
		st.PlayerDoomRounds = byte(n)
	} else {
		st.EnemyDoomRounds = byte(n)
	}
}

// tickDoom 回合末递减；归零且自身存活则秒杀对方。
func tickDoom(st *BattleState) {
	if st == nil {
		return
	}
	if st.PlayerDoomRounds > 0 {
		st.PlayerDoomRounds--
		if st.PlayerDoomRounds == 0 && st.PlayerHP > 0 {
			st.EnemyHP = 0
		}
	}
	if st.EnemyDoomRounds > 0 {
		st.EnemyDoomRounds--
		if st.EnemyDoomRounds == 0 && st.EnemyHP > 0 {
			st.PlayerHP = 0
		}
	}
}

// tickDoomPvP PvP：各自 PlayerDoomRounds 到期且自身存活则秒杀对方。
func tickDoomPvP(a, b *BattleState) {
	if a == nil || b == nil {
		return
	}
	if a.PlayerDoomRounds > 0 {
		a.PlayerDoomRounds--
		if a.PlayerDoomRounds == 0 && a.PlayerHP > 0 {
			b.PlayerHP = 0
			a.EnemyHP = 0
		}
	}
	if b.PlayerDoomRounds > 0 {
		b.PlayerDoomRounds--
		if b.PlayerDoomRounds == 0 && b.PlayerHP > 0 {
			a.PlayerHP = 0
			b.EnemyHP = 0
		}
	}
}


// beginCharge SideEffect 17：本回合蓄力，下回合自动以该技能出击（威力×2）。
func beginCharge(st *BattleState, skillID uint32, d *tableloader.SkillDef, playerIsAtk bool) bool {
	if st == nil || d == nil || !skillHasSideEffect(d, 17) || skillID == 0 {
		return false
	}
	if playerIsAtk {
		st.PlayerChargeSkill = skillID
	} else {
		st.EnemyChargeSkill = skillID
	}
	return true
}

// takeChargeSkill 取出上回合蓄力完成的技能并清空（释放回合威力×2 由调用方处理）。
func takeChargeSkill(st *BattleState, playerIsAtk bool) uint32 {
	if st == nil {
		return 0
	}
	if playerIsAtk {
		id := st.PlayerChargeSkill
		if id == 0 {
			return 0
		}
		st.PlayerChargeSkill = 0
		st.PlayerChargeReady = false
		return id
	}
	id := st.EnemyChargeSkill
	if id == 0 {
		return 0
	}
	st.EnemyChargeSkill = 0
	st.EnemyChargeReady = false
	return id
}

// applyOnKOEffects SideEffect 66/67/158：击杀时回血 / 标记下只敌宠削血 / 自身能力上升。
func applyOnKOEffects(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, foeHPBefore uint32) {
	if st == nil || d == nil || foeHPBefore == 0 {
		return
	}
	// 仅在本次攻击导致击杀时
	foeDead := false
	if playerIsAtk {
		foeDead = st.EnemyHP == 0
	} else {
		foeDead = st.PlayerHP == 0
	}
	if !foeDead {
		return
	}
	applyOnKOSelfStage(st, d, playerIsAtk, foeHPBefore)
	applyTransferBoostsOnKO421(st, d, playerIsAtk, foeHPBefore)
	if skillHasSideEffect(d, 66) {
		args := parseSideEffectArgs(d.SideEffectArg)
		denom := 3
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
			st.PlayerHP += heal
			if st.PlayerHP > st.PlayerMaxHP {
				st.PlayerHP = st.PlayerMaxHP
			}
		} else {
			heal := st.EnemyMaxHP / uint32(denom)
			if heal < 1 {
				heal = 1
			}
			st.EnemyHP += heal
			if st.EnemyHP > st.EnemyMaxHP {
				st.EnemyHP = st.EnemyMaxHP
			}
		}
	}
	if skillHasSideEffect(d, 67) {
		args := parseSideEffectArgs(d.SideEffectArg)
		denom := 3
		if len(args) >= 1 && args[0] > 0 {
			denom = args[0]
		}
		if denom < 1 {
			denom = 1
		}
		if playerIsAtk {
			st.EnemyNextEnterCutDenom = byte(denom)
		} else {
			st.PlayerNextEnterCutDenom = byte(denom)
		}
	}
}

// applySacrificeEffects SideEffect 59/71：献祭自身，标记下场精灵加成。
func applySacrificeEffects(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool) {
	if st == nil || d == nil {
		return
	}
	if skillHasSideEffect(d, 59) {
		args := parseSideEffectArgs(d.SideEffectArg)
		// 默认特攻+特防各 +1；Arg 可写 stageIndex 列表
		boost := [5]int8{}
		if len(args) == 0 {
			boost[stageSA], boost[stageSD] = 1, 1
		} else {
			for _, idx := range args {
				if idx >= 0 && idx <= stageSpd {
					boost[idx]++
				}
			}
		}
		if playerIsAtk {
			st.PlayerHP = 0
			st.PlayerNextStageBoost = boost
		} else {
			st.EnemyHP = 0
			st.EnemyNextStageBoost = boost
		}
	}
	if skillHasSideEffect(d, 71) {
		if playerIsAtk {
			st.PlayerHP = 0
			st.PlayerNextMustCritRounds = 2
		} else {
			st.EnemyHP = 0
			st.EnemyNextMustCritRounds = 2
		}
	}
}

// applyEnterPetPending 换宠入场时应用 59/67/71 遗留效果。
func applyEnterPetPending(st *BattleState, playerSide bool) {
	if st == nil {
		return
	}
	if playerSide {
		for i := range st.PlayerNextStageBoost {
			if st.PlayerNextStageBoost[i] != 0 {
				st.PlayerStages[i] = int8(clampStage(int(st.PlayerStages[i]) + int(st.PlayerNextStageBoost[i])))
			}
		}
		st.PlayerNextStageBoost = [5]int8{}
		if st.PlayerNextMustCritRounds > 0 {
			st.PlayerBuff.MustCritRounds = st.PlayerNextMustCritRounds
			st.PlayerNextMustCritRounds = 0
		}
		if st.PlayerNextEnterCutDenom > 0 && st.PlayerMaxHP > 0 {
			cut := st.PlayerMaxHP / uint32(st.PlayerNextEnterCutDenom)
			if cut < 1 {
				cut = 1
			}
			_ = applyDamage(&st.PlayerHP, cut)
			st.PlayerNextEnterCutDenom = 0
		}
		if st.PlayerNextEnterAtkDefBoost {
			st.PlayerStages[stageAtk] = int8(clampStage(int(st.PlayerStages[stageAtk]) + 1))
			st.PlayerStages[stageDef] = int8(clampStage(int(st.PlayerStages[stageDef]) + 1))
			st.PlayerNextEnterAtkDefBoost = false
		}
		return
	}
	for i := range st.EnemyNextStageBoost {
		if st.EnemyNextStageBoost[i] != 0 {
			st.EnemyStages[i] = int8(clampStage(int(st.EnemyStages[i]) + int(st.EnemyNextStageBoost[i])))
		}
	}
	st.EnemyNextStageBoost = [5]int8{}
	if st.EnemyNextMustCritRounds > 0 {
		st.EnemyBuff.MustCritRounds = st.EnemyNextMustCritRounds
		st.EnemyNextMustCritRounds = 0
	}
	if st.EnemyNextEnterCutDenom > 0 && st.EnemyMaxHP > 0 {
		cut := st.EnemyMaxHP / uint32(st.EnemyNextEnterCutDenom)
		if cut < 1 {
			cut = 1
		}
		_ = applyDamage(&st.EnemyHP, cut)
		st.EnemyNextEnterCutDenom = 0
	}
}

// applyFirstStrikeReflect SideEffect 73：先手攻击后，本回合受伤则 2 倍反击。
func applyFirstStrikeReflect(st *BattleState, d *tableloader.SkillDef, playerIsAtk, wentFirst bool) {
	if st == nil || d == nil || !skillHasSideEffect(d, 73) || !wentFirst {
		return
	}
	if playerIsAtk {
		st.PlayerBuff.CounterDouble = true
	} else {
		st.EnemyBuff.CounterDouble = true
	}
}

func tryCounterDoubleReflect(def *battleBuff, lost uint32, atkHP *uint32) {
	if def == nil || !def.CounterDouble || lost == 0 || atkHP == nil {
		return
	}
	_ = applyDamage(atkHP, lost*2)
	def.CounterDouble = false
}
