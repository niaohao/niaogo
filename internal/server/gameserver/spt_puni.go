package gameserver

import (
	"log"

	"niaohao/server/internal/tableloader"
)

// 谱尼封印战斗机制（对照参考服 sptboss + battle_effect_boss*）。
const (
	puniRegionVoid    = 1 // 虚无
	puniRegionElement = 2 // 元素
	puniRegionEnergy  = 3 // 能量
	puniRegionLife    = 4 // 生命
	puniRegionSamsara = 5 // 轮回
	puniRegionEternal = 6 // 永恒
	puniRegionHoly    = 7 // 圣洁
	puniRegionTrue    = 8 // 真身

	puniEnergyDmgCap      uint32 = 100
	puniLifeRegenPerRound uint32 = 2000
	puniSuperHP           int    = 65000
	puniTimeSenseSkillID  uint32 = 20300 // 时空感应：破真身虚无
)

func isPuniBattle(st *BattleState) bool {
	return st != nil && isPuniSealBoss(st.MapID, st.EnemyID, st.BossRegion)
}

func hasClearedAllPuniSeals(s *Server, uid int64) bool {
	return s != nil && s.maxPuniLvOf(uid) >= 7
}

func puniLivesForRegion(region uint32) int {
	switch region {
	case puniRegionSamsara:
		return 2
	case puniRegionTrue:
		return 6
	default:
		return 0
	}
}

func puniTrueFormLifeMaxHP(life int) int {
	switch life {
	case 1:
		return 7000
	case 2, 3:
		return 8000
	case 4:
		return 10000
	case 5:
		return 20000
	case 6:
		return 65000
	default:
		return 0
	}
}

func puniEnemyPPInfinite(st *BattleState) bool {
	if !isPuniBattle(st) {
		return false
	}
	switch st.BossRegion {
	case puniRegionEternal, puniRegionHoly:
		return true
	case puniRegionTrue:
		if st.PuniTotalLives == 6 {
			// 真身第 2–4 命有限 PP
			return st.PuniCurrentLife < 2 || st.PuniCurrentLife > 4
		}
		return true // 厉害谱尼
	default:
		return false
	}
}

func enemyHasInfinitePP(st *BattleState) bool {
	if st == nil {
		return false
	}
	if isInfinitePPBoss(st.EnemyID) {
		return true
	}
	return puniEnemyPPInfinite(st)
}

// puniControlVulnerable 元素/能量封印（及真身对应命）可吃控制。
func puniControlVulnerable(st *BattleState) bool {
	if !isPuniBattle(st) {
		return false
	}
	if st.BossRegion == puniRegionElement || st.BossRegion == puniRegionEnergy {
		return true
	}
	if st.BossRegion == puniRegionTrue && st.PuniTotalLives == 6 {
		return st.PuniCurrentLife == 2 || st.PuniCurrentLife == 3
	}
	return false
}

func canApplyEnemyBattleStatus(st *BattleState, eid int) bool {
	if st == nil {
		return canApplyEnemyStatus(0, eid)
	}
	if isStatusImmuneBoss(st.EnemyID) {
		return false
	}
	switch eid {
	case 10, 14, 15, 16, 20, 22:
		if puniControlVulnerable(st) {
			return true
		}
		if isControlImmuneBoss(st.EnemyID) {
			return false
		}
	}
	return true
}

func bossPriorityBonusBattle(st *BattleState) int {
	if st == nil {
		return 0
	}
	if isPuniBattle(st) && st.BossRegion == puniRegionTrue && st.PuniTotalLives == 0 {
		return 20 // 厉害谱尼
	}
	return bossPriorityBonus(st.EnemyID, st.EnemyHP, st.EnemyMaxHP)
}

// initPuniBattleOnOpen 2411 开战后初始化多命 / 厉害谱尼血量。
func (s *Server) initPuniBattleOnOpen(uid uint32, st *BattleState) {
	if !isPuniBattle(st) {
		return
	}
	st.PuniTotalLives = 0
	st.PuniCurrentLife = 0
	st.PuniElementLastType = 0
	st.PuniTrueFormSuppressed = false

	total := puniLivesForRegion(st.BossRegion)
	if st.BossRegion == puniRegionTrue && total == 6 && !hasClearedAllPuniSeals(s, int64(uid)) {
		total = 0 // 厉害谱尼
	}
	st.PuniTotalLives = total
	if total > 0 {
		st.PuniCurrentLife = 1
	}
	if st.BossRegion == puniRegionTrue {
		if total == 6 {
			if hp := puniTrueFormLifeMaxHP(1); hp > 0 {
				st.EnemyMaxHP = uint32(hp)
				st.EnemyHP = st.EnemyMaxHP
			}
		} else {
			st.EnemyMaxHP = uint32(puniSuperHP)
			st.EnemyHP = st.EnemyMaxHP
		}
	}
}

func puniHasVoid(st *BattleState) bool {
	if !isPuniBattle(st) {
		return false
	}
	if st.BossRegion == puniRegionVoid {
		return true
	}
	return st.BossRegion == puniRegionTrue && st.PuniTotalLives == 6 &&
		st.PuniCurrentLife == 1 && !st.PuniTrueFormSuppressed
}

func puniHasElement(st *BattleState) bool {
	if !isPuniBattle(st) {
		return false
	}
	if st.BossRegion == puniRegionElement {
		return true
	}
	return st.BossRegion == puniRegionTrue && st.PuniTotalLives == 6 && st.PuniCurrentLife == 2
}

func puniHasEnergy(st *BattleState) bool {
	if !isPuniBattle(st) {
		return false
	}
	if st.BossRegion == puniRegionEnergy {
		return true
	}
	return st.BossRegion == puniRegionTrue && st.PuniTotalLives == 6 && st.PuniCurrentLife == 3
}

func puniHasLifeRegen(st *BattleState) bool {
	if !isPuniBattle(st) || st.EnemyHP == 0 {
		return false
	}
	if st.BossRegion == puniRegionLife {
		return true
	}
	return st.BossRegion == puniRegionTrue && st.PuniTotalLives == 6 && st.PuniCurrentLife == 4
}

func puniHasEternalHalf(st *BattleState) bool {
	if !isPuniBattle(st) {
		return false
	}
	if st.BossRegion == puniRegionEternal || st.BossRegion == puniRegionHoly {
		return true
	}
	return st.BossRegion == puniRegionTrue && st.PuniTotalLives == 6 && st.PuniCurrentLife == 5
}

func skillIsMustHit(d *tableloader.SkillDef) bool {
	return d != nil && d.MustHit == 1
}

// applyPuniRoundStart 回合开始：生命回血 / 真身第 6 命低血回满。
func applyPuniRoundStart(st *BattleState) {
	if !isPuniBattle(st) || st.EnemyHP == 0 {
		return
	}
	if puniHasLifeRegen(st) {
		st.EnemyHP += puniLifeRegenPerRound
		if st.EnemyHP > st.EnemyMaxHP {
			st.EnemyHP = st.EnemyMaxHP
		}
	}
	if st.BossRegion == puniRegionTrue && st.PuniTotalLives == 6 && st.PuniCurrentLife == 6 &&
		st.EnemyHP > 0 && st.EnemyHP < 1000 {
		st.EnemyHP = st.EnemyMaxHP
	}
}

// applyPuniOnPlayerSkillHit 命中后、扣血前：虚无 / 元素 / 能量 / 时空感应。
// 返回 (dmg, hit)。
func applyPuniOnPlayerSkillHit(st *BattleState, skillID uint32, d *tableloader.SkillDef, dmg uint32, hit bool) (uint32, bool) {
	if !isPuniBattle(st) || !hit {
		return dmg, hit
	}
	// 时空感应破真身虚无
	if skillID == puniTimeSenseSkillID && st.BossRegion == puniRegionTrue &&
		st.PuniTotalLives == 6 && st.PuniCurrentLife == 1 {
		st.PuniTrueFormSuppressed = true
	}

	if puniHasVoid(st) {
		if !skillIsMustHit(d) && !mustHitFromBuff(&st.PlayerBuff) {
			return 0, false
		}
	}

	if puniHasElement(st) && d != nil && d.Category != 4 {
		if d.Type != 12 && d.Type != 13 {
			return 0, hit
		}
		if st.PuniElementLastType == byte(d.Type) {
			return 0, hit
		}
		st.PuniElementLastType = byte(d.Type)
	}

	if puniHasEnergy(st) && dmg > puniEnergyDmgCap {
		dmg = puniEnergyDmgCap
		st.PlayerHP = 0
	}
	return dmg, hit
}

// applyPuniEternalHalf 永恒/圣洁/真身第5命：最终伤害 /2。
func applyPuniEternalHalf(st *BattleState, dmg uint32) uint32 {
	if dmg == 0 || !puniHasEternalHalf(st) {
		return dmg
	}
	dmg /= 2
	if dmg < 1 && st.EnemyHP > 0 {
		dmg = 1
	}
	return dmg
}

func resetEnemyStateOnPuniLifeSwitch(st *BattleState) {
	if st == nil {
		return
	}
	st.EnemyStages = [5]int8{}
	st.EnemyStatus = battleStatus{}
	st.EnemyBuff = battleBuff{}
	st.EnemyChargeSkill = 0
	st.EnemyChargeReady = false
	st.EnemyDoomRounds = 0
	st.EnemySkillFail = false
	st.EnemyCritBonusRounds = 0
	st.EnemyConsecSkillID = 0
	st.EnemyConsecSkillCount = 0
	st.EnemyLastTaken = 0
	st.PuniElementLastType = 0
	st.PuniTrueFormSuppressed = false
	if shouldBossInnateAtkPlus2(st.EnemyID) {
		st.EnemyStages[stageAtk] = 2
	}
}

// sendEnemyLifeSwitch2407 谱尼等多命切命后推 ChangePetInfo（userID=0）。
func (s *Server) sendEnemyLifeSwitch2407(c *Client, uid uint32, st *BattleState) {
	if st == nil {
		return
	}
	name := st.EnemyName
	if name == "" {
		if s.cfg.Catalog != nil {
			name = s.cfg.Catalog.PetNameOf(st.EnemyID)
		}
		if name == "" {
			name = "谱尼"
		}
	}
	out := buildChangePetInfo(0, st.EnemyID, name, uint32(st.EnemyLevel), st.EnemyHP, st.EnemyMaxHP, 0)
	s.send(c, 2407, uid, 0, out)
	log.Printf("[CMD] OK     2407 CHANGE_PET UID=%d (enemy lifeSwitch) pet=%d hp=%d/%d life=%d/%d",
		uid, st.EnemyID, st.EnemyHP, st.EnemyMaxHP, st.PuniCurrentLife, st.PuniTotalLives)
}

// tryPuniLifeSwitch 敌方致死时切命；返回 true 表示已切命（战斗继续）。
func tryPuniLifeSwitch(st *BattleState) bool {
	if !isPuniBattle(st) || st.EnemyHP > 0 {
		return false
	}
	// 轮回：2 命
	if st.BossRegion == puniRegionSamsara && st.PuniTotalLives == 2 && st.PuniCurrentLife < 2 {
		st.PuniCurrentLife = 2
		st.EnemyHP = st.EnemyMaxHP
		resetEnemyStateOnPuniLifeSwitch(st)
		return true
	}
	// 真身 6 命
	if st.BossRegion == puniRegionTrue && st.PuniTotalLives == 6 && st.PuniCurrentLife < 6 {
		st.PuniCurrentLife++
		if hp := puniTrueFormLifeMaxHP(st.PuniCurrentLife); hp > 0 {
			st.EnemyMaxHP = uint32(hp)
			st.EnemyHP = st.EnemyMaxHP
		} else {
			st.EnemyHP = st.EnemyMaxHP
		}
		resetEnemyStateOnPuniLifeSwitch(st)
		return true
	}
	// 厉害谱尼：致死回满
	if st.BossRegion == puniRegionTrue && st.PuniTotalLives == 0 {
		st.EnemyHP = st.EnemyMaxHP
		resetEnemyStateOnPuniLifeSwitch(st)
		return true
	}
	return false
}

// applyPuniSuperLowHPOneShot 厉害谱尼：敌方 HP<20000 时秒杀我方。
func applyPuniSuperLowHPOneShot(st *BattleState, enemyDmg uint32, enemyHit bool) uint32 {
	if !enemyHit || !isPuniBattle(st) {
		return enemyDmg
	}
	if st.BossRegion != puniRegionTrue || st.PuniTotalLives != 0 {
		return enemyDmg
	}
	if st.EnemyHP >= 20000 {
		return enemyDmg
	}
	if st.PlayerHP > 0 {
		return st.PlayerHP
	}
	return enemyDmg
}
