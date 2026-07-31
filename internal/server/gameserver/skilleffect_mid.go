package gameserver

import (
	"math/rand"

	"niaohao/server/internal/tableloader"
)

// sideEffectChanceMulDamage SideEffect 88：n% 几率伤害为 m 倍。
func sideEffectChanceMulDamage(d *tableloader.SkillDef, dmg uint32) uint32 {
	if d == nil || dmg == 0 || !skillHasSideEffect(d, 88) {
		return dmg
	}
	args := parseSideEffectArgs(d.SideEffectArg)
	chance, mul := 20, 2
	if len(args) >= 1 && args[0] > 0 {
		chance = args[0]
	}
	if len(args) >= 2 && args[1] > 0 {
		mul = args[1]
	}
	if chance > 100 {
		chance = 100
	}
	if mul < 2 {
		mul = 2
	}
	if rand.Intn(100) >= chance {
		return dmg
	}
	return dmg * uint32(mul)
}

// foeStatusDamageMul SideEffect 96/97：对手烧伤/冻伤时伤害翻倍。
func foeStatusDamageMul(d *tableloader.SkillDef, foe *battleStatus, dmg uint32) uint32 {
	if d == nil || foe == nil || dmg == 0 {
		return dmg
	}
	if skillHasSideEffect(d, 96) && foe.Burn {
		dmg *= 2
	}
	if skillHasSideEffect(d, 97) && foe.Freeze {
		dmg *= 2
	}
	return dmg
}

// lowHPDamageScale SideEffect 100：自身体力越少伤害越高（满血 1x，空血约 2x）。
func lowHPDamageScale(d *tableloader.SkillDef, selfHP, selfMax, dmg uint32) uint32 {
	if d == nil || dmg == 0 || selfMax == 0 || !skillHasSideEffect(d, 100) {
		return dmg
	}
	missing := 1.0 - float64(selfHP)/float64(selfMax)
	if missing < 0 {
		missing = 0
	}
	if missing > 1 {
		missing = 1
	}
	return uint32(float64(dmg) * (1.0 + missing))
}

// sacrificeHalfEqualDamage SideEffect 80：自损一半最大体力，造成等量伤害。
func sacrificeHalfEqualDamage(d *tableloader.SkillDef, selfHP, selfMax uint32) (dmg, selfLoss uint32, ok bool) {
	if d == nil || !skillHasSideEffect(d, 80) || selfMax == 0 {
		return 0, 0, false
	}
	loss := selfMax / 2
	if loss < 1 {
		loss = 1
	}
	if loss > selfHP {
		loss = selfHP
	}
	return loss, loss, true
}

// sleepCritExtra SideEffect 95：对手睡眠时暴击率 +arg/16。
func sleepCritExtra(d *tableloader.SkillDef, foeSleep bool) int {
	if d == nil || !foeSleep || !skillHasSideEffect(d, 95) {
		return 0
	}
	args := parseSideEffectArgs(d.SideEffectArg)
	n := 4
	if len(args) >= 1 && args[0] > 0 {
		n = args[0]
	}
	return n
}

// restoreAllSkillPP SideEffect 87。
func restoreAllSkillPP(skills [][2]uint32, skillOf func(int) *tableloader.SkillDef) {
	if skillOf == nil {
		return
	}
	for i := range skills {
		sid := int(skills[i][0])
		if sid <= 0 {
			continue
		}
		if d := skillOf(sid); d != nil && d.MaxPP > 0 {
			skills[i][1] = uint32(d.MaxPP)
		}
	}
}

// stealPositiveStages SideEffect 85：对手能力提升转到自己。
func stealPositiveStages(self, foe *[5]int8) {
	if self == nil || foe == nil {
		return
	}
	for i := range foe {
		if foe[i] > 0 {
			self[i] = int8(clampStage(int(self[i]) + int(foe[i])))
			foe[i] = 0
		}
	}
}

// applyHalfHPStageBoost SideEffect 79：损失 1/2 体力，全能力 +1。
func applyHalfHPStageBoost(hp *uint32, maxHP uint32, stages *[5]int8, _ []int, argOff int) int {
	if hp == nil || stages == nil || maxHP == 0 {
		return argOff
	}
	loss := maxHP / 2
	if loss < 1 {
		loss = 1
	}
	_ = applyDamage(hp, loss)
	for i := range stages {
		stages[i] = int8(clampStage(int(stages[i]) + 1))
	}
	return argOff
}

// mustHitFromBuff SideEffect 81。
func mustHitFromBuff(b *battleBuff) bool {
	return b != nil && b.MustHitRounds > 0
}

// specMissForced SideEffect 86：特殊攻击对自身必 miss。
func specMissForced(def *battleBuff, sk *tableloader.SkillDef) bool {
	if def == nil || def.SpecMissRounds == 0 || sk == nil {
		return false
	}
	return sk.Category == 2
}

// applyVampOnDamage SideEffect 89：造成伤害后按比例回血。
func applyVampOnDamage(atk *battleBuff, actual uint32, hp, maxHP *uint32) {
	if atk == nil || actual == 0 || hp == nil || maxHP == nil || atk.VampRounds == 0 || atk.VampDenom < 1 {
		return
	}
	heal := actual / uint32(atk.VampDenom)
	if heal < 1 {
		heal = 1
	}
	*hp += heal
	if *hp > *maxHP {
		*hp = *maxHP
	}
}

// tryOnHitStatus SideEffect 84/92/108：受物理攻击时概率麻痹/冻伤/烧伤对手。
func tryOnHitStatus(def *battleBuff, atkSkill *tableloader.SkillDef, atkStatus *battleStatus) {
	if def == nil || atkStatus == nil || atkSkill == nil {
		return
	}
	cat := atkSkill.Category
	if cat == 0 {
		cat = 1
	}
	if cat != 1 {
		return
	}
	if def.OnHitParaRounds > 0 && def.OnHitParaChance > 0 && rand.Intn(100) < int(def.OnHitParaChance) {
		setStatus(atkStatus, 10)
	}
	if def.OnHitFreezeRounds > 0 && def.OnHitFreezeChance > 0 && rand.Intn(100) < int(def.OnHitFreezeChance) {
		setStatus(atkStatus, 14)
	}
	if def.OnHitBurnRounds > 0 && def.OnHitBurnChance > 0 && rand.Intn(100) < int(def.OnHitBurnChance) {
		setStatus(atkStatus, 12)
	}
}

// applyTypeSwapOrCopy SideEffect 55/56。
func applyTypeSwapOrCopy(st *BattleState, d *tableloader.SkillDef, playerIsAtk bool, args []int, argOff int) int {
	if st == nil || d == nil {
		return argOff
	}
	rounds := 5
	if argOff < len(args) && args[argOff] > 0 {
		rounds = args[argOff]
		argOff++
	}
	r := clampRounds(rounds)
	if skillHasSideEffect(d, 55) {
		if st.PlayerTypeOverrideRounds == 0 {
			st.PlayerTypeSaved = byte(st.PlayerType)
		}
		if st.EnemyTypeOverrideRounds == 0 {
			st.EnemyTypeSaved = byte(st.EnemyType)
		}
		st.PlayerType, st.EnemyType = st.EnemyType, st.PlayerType
		st.PlayerTypeOverrideRounds = r
		st.EnemyTypeOverrideRounds = r
		return argOff
	}
	if skillHasSideEffect(d, 56) {
		if playerIsAtk {
			if st.PlayerTypeOverrideRounds == 0 {
				st.PlayerTypeSaved = byte(st.PlayerType)
			}
			st.PlayerType = st.EnemyType
			st.PlayerTypeOverrideRounds = r
		} else {
			if st.EnemyTypeOverrideRounds == 0 {
				st.EnemyTypeSaved = byte(st.EnemyType)
			}
			st.EnemyType = st.PlayerType
			st.EnemyTypeOverrideRounds = r
		}
	}
	return argOff
}

func tickTypeOverride(st *BattleState) {
	if st == nil {
		return
	}
	if st.PlayerTypeOverrideRounds > 0 {
		st.PlayerTypeOverrideRounds--
		if st.PlayerTypeOverrideRounds == 0 && st.PlayerTypeSaved != 0 {
			st.PlayerType = int(st.PlayerTypeSaved)
			st.PlayerTypeSaved = 0
		}
	}
	if st.EnemyTypeOverrideRounds > 0 {
		st.EnemyTypeOverrideRounds--
		if st.EnemyTypeOverrideRounds == 0 && st.EnemyTypeSaved != 0 {
			st.EnemyType = int(st.EnemyTypeSaved)
			st.EnemyTypeSaved = 0
		}
	}
}
