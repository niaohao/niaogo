package gameserver

import (
	"math/rand"

	"niaohao/server/internal/tableloader"
)

// applyTraitOutgoingDamage 进攻侧特性：系伤加成(1006-1021)、瞬杀(1027)。
// instantKill=true 时返回满血伤害（由调用方 cap 到敌方 HP）。
func applyTraitOutgoingDamage(trait int, skill *tableloader.SkillDef, dmg uint32) (out uint32, instantKill bool) {
	out = dmg
	if trait <= 0 || dmg == 0 {
		return out, false
	}
	if skill != nil && skill.Category != 4 && trait >= 1006 && trait <= 1021 {
		wantType := trait - 1005
		if skill.Type == wantType {
			out = out * 105 / 100
			if out < 1 {
				out = 1
			}
		}
	}
	if skill != nil && skill.Category != 4 && trait == 1027 && rand.Intn(100) < 3 {
		return out, true
	}
	return out, false
}

// applyTraitIncomingDamage 防守侧：坚硬(1024) 减伤 5%；顽强(1026) 致死留 1。
func applyTraitIncomingDamage(trait int, hpBefore, dmg uint32) uint32 {
	if dmg == 0 {
		return 0
	}
	if trait == 1024 {
		dmg = dmg * 95 / 100
		if dmg < 1 {
			dmg = 1
		}
	}
	if trait == 1026 && hpBefore > 1 && dmg >= hpBefore && rand.Intn(100) < 3 {
		return hpBefore - 1
	}
	return dmg
}

// traitCritBonusSixteenths 会心(1023)：+1/16。
func traitCritBonusSixteenths(trait int) int {
	if trait == 1023 {
		return 1
	}
	return 0
}

// traitHitAccuracyBonus 精准(1022)：命中 +5。
func traitHitAccuracyBonus(trait int) int {
	if trait == 1022 {
		return 5
	}
	return 0
}

// traitEvasionBonus 回避(1025)：被命中 −5（提高对方闪避等效）。
func traitEvasionBonus(trait int) int {
	if trait == 1025 {
		return 1 // 用 evaStage+1 ≈ +25% 太强；改为命中率直接 -5
	}
	return 0
}

// applyTraitHitChance 在基础命中率上叠加精准/回避。
func applyTraitHitChance(baseChance, atkTrait, defTrait int) int {
	chance := baseChance + traitHitAccuracyBonus(atkTrait)
	if defTrait == 1025 {
		chance -= 5
	}
	if chance < 1 {
		chance = 1
	}
	if chance > 100 {
		chance = 100
	}
	return chance
}

// traitDrainHeal 汲取(1039)：回复造成伤害的 8%。
func traitDrainHeal(trait int, dealt uint32) uint32 {
	if trait != 1039 || dealt == 0 {
		return 0
	}
	h := dealt * 8 / 100
	if h < 1 {
		h = 1
	}
	return h
}

// tryTraitRecoverOnLowHP 回神(1028)：≤1/8 最大体力时 3% 回满。
func tryTraitRecoverOnLowHP(trait int, hp, maxHP uint32) uint32 {
	if trait != 1028 || maxHP == 0 || hp == 0 {
		return hp
	}
	if hp*8 > maxHP {
		return hp
	}
	if rand.Intn(100) < 3 {
		return maxHP
	}
	return hp
}

// applyTraitReactiveOnHit 被命中后触发（PetEffectXML Stat=1）：
// 1029-1034 物攻反常 3%；1035-1040 特攻降敌能力 5%；1041-1045 任意攻击升己能力 5%。
// defStages=被打方，atkStages/atkStatus=进攻方。
func applyTraitReactiveOnHit(defTrait int, atkSkill *tableloader.SkillDef, dealt uint32, defStages, atkStages *[5]int8, atkStatus *battleStatus) {
	if defTrait < 1029 || defTrait > 1045 || defTrait == 1039 || atkSkill == nil {
		return
	}
	cat := atkSkill.Category
	if cat == 4 {
		return
	}

	// 1029-1034：受到物攻且造成伤害 → 3% 给进攻方挂异常
	if defTrait >= 1029 && defTrait <= 1034 {
		if cat != 1 || dealt == 0 || atkStatus == nil {
			return
		}
		if rand.Intn(100) >= 3 {
			return
		}
		setTraitReflectStatus(atkStatus, defTrait)
		return
	}

	// 1035-1038 / 1040：受到特攻 → 5% 降进攻方对应能力 1 级
	if (defTrait >= 1035 && defTrait <= 1038) || defTrait == 1040 {
		if cat != 2 || atkStages == nil {
			return
		}
		if rand.Intn(100) >= 5 {
			return
		}
		stat := traitStatIndexForDown(defTrait)
		if stat < 0 {
			return
		}
		atkStages[stat] = int8(clampStage(int(atkStages[stat]) - 1))
		return
	}

	// 1041-1045：受到物/特攻 → 5% 升被打方对应能力 1 级
	if defTrait >= 1041 && defTrait <= 1045 {
		if (cat != 1 && cat != 2) || defStages == nil {
			return
		}
		if rand.Intn(100) >= 5 {
			return
		}
		stat := defTrait - 1041 // 0攻 1防 2特攻 3特防 4速度
		if stat < 0 || stat > stageSpd {
			return
		}
		defStages[stat] = int8(clampStage(int(defStages[stat]) + 1))
	}
}

func setTraitReflectStatus(st *battleStatus, trait int) {
	if st == nil {
		return
	}
	switch trait {
	case 1029:
		st.Para = true
	case 1030:
		st.Poison = true
	case 1031:
		st.Burn = true
	case 1032:
		st.Freeze = true
	case 1033:
		st.Fear = true
	case 1034:
		st.Sleep = true
	}
}

func traitStatIndexForDown(trait int) int {
	switch trait {
	case 1035:
		return stageAtk
	case 1036:
		return stageDef
	case 1037:
		return stageSA
	case 1038:
		return stageSD
	case 1040:
		return stageSpd
	default:
		return -1
	}
}

// applyPvEPlayerTraitOnHit 玩家被敌方命中后结算防守特性。
func applyPvEPlayerTraitOnHit(st *BattleState, _ uint32, dealt uint32, skill *tableloader.SkillDef) {
	if st == nil || !IsValidPetTrait(st.PlayerTrait) || skill == nil {
		return
	}
	applyTraitReactiveOnHit(st.PlayerTrait, skill, dealt, &st.PlayerStages, &st.EnemyStages, &st.EnemyStatus)
}
