package gameserver

// SPT / 地图 Boss「机制特性」（对照参考服 sptboss StatusImmune 等）。
// 注意：这不是融合 NewSe（1006–1045），而是按 petID 的战斗规则。

// StatusImmuneBossIDs 免疫全部异常（烧/冻/睡/麻/毒/畏缩等）。
var statusImmuneBossIDs = map[int]bool{
	70: true, 216: true, 264: true, 261: true,
	798: true, 804: true, 715: true,
	501: true, 502: true, 503: true, 5033: true,
	1000: true, 124: true, 125: true,
	393: true, 875: true, 957: true, 1723: true,
}

// StatDropImmuneBossIDs 免疫能力下降（负向 stage）。
var statDropImmuneBossIDs = map[int]bool{
	88: true, 113: true, 132: true, 216: true, 261: true,
	274: true, 391: true, 300: true, 715: true, 804: true,
	501: true, 502: true, 1000: true, 124: true, 125: true,
	589: true, 1337: true, 503: true, 5033: true, 393: true,
	875: true, 957: true, 1723: true,
}

// InfinitePPBossIDs 敌方技能不扣 PP。
var infinitePPBossIDs = map[int]bool{
	70: true, 187: true, 264: true, 261: true, 132: true,
	804: true, 5033: true, 875: true,
}

// PriorityBonusBossIDs 技能等效先制 +6（本服简化：有加成则敌方先手）。
var priorityBonusBossIDs = map[int]bool{
	70: true, 261: true, 503: true, 875: true,
}

// HalfHPOneShotBossIDs 半血后先制+6且秒杀我方当前宠。
var halfHPOneShotBossIDs = map[int]bool{
	187: true, // 魔狮迪露
}

// DamageTakenMultiplierBossIDs 受到我方攻击伤害倍数（仅攻击伤害，不含异常）。
var damageTakenMultiplierBossIDs = map[int]int{
	187: 10, // 魔狮迪露 ×10
}

func isStatusImmuneBoss(petID int) bool {
	return statusImmuneBossIDs[petID]
}

func isControlImmuneBoss(petID int) bool {
	if petID == 50 {
		// 阿克希亚：雷伊特训需可麻痹，不免疫控制
		return false
	}
	if isStatusImmuneBoss(petID) {
		return true
	}
	// 凡在本服 Boss 表内的均免疫控制类异常
	return isListedMapBossPet(petID)
}

func isStatDropImmuneBoss(petID int) bool {
	return statDropImmuneBossIDs[petID]
}

func isInfinitePPBoss(petID int) bool {
	return infinitePPBossIDs[petID]
}

func bossPriorityBonus(petID int, curHP, maxHP uint32) int {
	if priorityBonusBossIDs[petID] {
		return 6
	}
	if halfHPOneShotBossIDs[petID] && maxHP > 0 && curHP*2 < maxHP {
		return 6
	}
	return 0
}

func isHalfHPOneShotBoss(petID int) bool {
	return halfHPOneShotBossIDs[petID]
}

func bossDamageTakenMultiplier(petID int) int {
	if n := damageTakenMultiplierBossIDs[petID]; n > 1 {
		return n
	}
	return 1
}

// applyBossHalfHPOneShot 魔狮等：半血后任意技能秒杀我方当前宠。
func applyBossHalfHPOneShot(st *BattleState, enemyDmg uint32, enemyHit bool) uint32 {
	if st == nil || !enemyHit {
		return enemyDmg
	}
	if !isHalfHPOneShotBoss(st.EnemyID) || st.EnemyMaxHP == 0 {
		return enemyDmg
	}
	if st.EnemyHP*2 >= st.EnemyMaxHP {
		return enemyDmg
	}
	if st.PlayerHP > 0 {
		return st.PlayerHP
	}
	return enemyDmg
}

func isListedMapBossPet(petID int) bool {
	if petID <= 0 {
		return false
	}
	for _, byParam := range mapBossByParam {
		for _, e := range byParam {
			if e.PetID == petID {
				return true
			}
		}
	}
	return false
}

// canApplyEnemyStatus 玩家对 Boss 上异常时是否允许（eid=SideEffect 状态码）。
func canApplyEnemyStatus(petID, eid int) bool {
	if isStatusImmuneBoss(petID) {
		return false
	}
	// 控制类：麻痹/冰冻/畏缩/睡眠/疲惫
	switch eid {
	case 10, 14, 15, 16, 20, 22:
		if isControlImmuneBoss(petID) {
			return false
		}
	}
	return true
}

// clampEnemyNegativeStages Boss 免疫能力下降：把负向 stage 抬回 0。
func clampEnemyNegativeStages(st *BattleState) {
	if st == nil || !isStatDropImmuneBoss(st.EnemyID) {
		return
	}
	for i := range st.EnemyStages {
		if st.EnemyStages[i] < 0 {
			st.EnemyStages[i] = 0
		}
	}
}

// shouldBossInnateAtkPlus2 对照参考服 sptboss.GetByPetID：经典 SPT / 首杀表 Boss 开局攻击等级 +2。
func shouldBossInnateAtkPlus2(petID int) bool {
	if petID <= 0 {
		return false
	}
	if _, ok := sptFirstKillByPetID[petID]; ok {
		return true
	}
	// 首杀表外、参考服仍给天生攻击+2 的 SPT
	switch petID {
	case 4150, 300, 166, 589, 672, 798, 804, 875, 925, 957, 1723, 1000, 5033:
		return true
	}
	return false
}

// applyBossInnateStages SPT Boss 开局强化（写入 EnemyStages，2504 battleLv 与伤害计算共用）。
// 暗黑第十一门三只按参考服定制：萨洛奇斯/奈尼狄亚防御+2，查迪斯攻击+2。
func applyBossInnateStages(st *BattleState) {
	if st == nil || st.EnemyID <= 0 || st.isPvP() {
		return
	}
	switch st.EnemyID {
	case 1403, 1400: // 萨洛奇斯 / 奈尼狄亚
		st.EnemyStages = [5]int8{}
		st.EnemyStages[stageDef] = 2
		return
	case 1397: // 查迪斯
		st.EnemyStages = [5]int8{}
		st.EnemyStages[stageAtk] = 2
		return
	}
	if shouldBossInnateAtkPlus2(st.EnemyID) {
		st.EnemyStages[stageAtk] = 2
	}
}

// applyBossOpenBattleRules 开战统一：固定血量 + 天生能力等级。
func applyBossOpenBattleRules(st *BattleState) {
	applyBossFixedHP(st)
	applyBossInnateStages(st)
}
