package gameserver

import "math/rand"

// calcHitChance 基础命中 + 命中/闪避等级差（每级 ±25，夹到 1~100）。
// 对齐参考 CalcHitChance；本服暂无闪避等级时 evaStage=0。
func calcHitChance(baseAccuracy, accStage, evaStage int) int {
	if baseAccuracy <= 0 {
		baseAccuracy = 100
	}
	stage := accStage - evaStage
	if stage > 6 {
		stage = 6
	}
	if stage < -6 {
		stage = -6
	}
	finalAcc := baseAccuracy + 25*stage
	if finalAcc < 1 {
		finalAcc = 1
	}
	if finalAcc > 100 {
		finalAcc = 100
	}
	return finalAcc
}

// checkSkillHit 按 SkillXML Accuracy / MustHit 判定；无表时默认命中。
func (s *Server) checkSkillHit(skillID uint32, accStage, evaStage int) bool {
	return s.checkSkillHitTrait(skillID, accStage, evaStage, 0, 0)
}

// checkSkillHitTrait 叠加进攻方精准(1022) / 防守方回避(1025)。
func (s *Server) checkSkillHitTrait(skillID uint32, accStage, evaStage, atkTrait, defTrait int) bool {
	d := s.skillDef(int(skillID))
	if d == nil {
		return true
	}
	if d.MustHit == 1 {
		return true
	}
	chance := applyTraitHitChance(calcHitChance(d.Accuracy, accStage, evaStage), atkTrait, defTrait)
	return rand.Intn(100) < chance
}
