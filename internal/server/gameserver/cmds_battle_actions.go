package gameserver

import (
	"encoding/binary"
	"log"
	"math/rand"
	"time"

	"niaohao/server/internal/store"
)

// handleChangePet CMD 2407：切换出战精灵 → ChangePetInfo(40B)；主动换宠后敌方反击 2505。
func (s *Server) handleChangePet(c *Client, uid uint32, body []byte) {
	reqCatch := uint32(0)
	if len(body) >= 4 {
		reqCatch = binary.BigEndian.Uint32(body[0:4])
	}
	st := s.battles.get(int64(uid))
	if st == nil || !st.Active {
		log.Printf("[CMD] OK     2407 CHANGE_PET UID=%d (no battle)", uid)
		s.send(c, 2407, uid, 0, buildChangePetInfo(uid, 1, "?", 5, 1, 1, 0))
		return
	}
	if st.isPvP() && st.PvPMode == pvpModeSingle && st.PlayerHP > 0 {
		s.send(c, 2407, uid, 0, buildChangePetInfo(uid, st.PlayerPetID, st.PlayerName,
			uint32(st.PlayerLevel), st.PlayerHP, st.PlayerMaxHP, st.PlayerCatchTime))
		s.sendAlert(int64(uid), "单挑模式不能换宠")
		return
	}

	var bag []store.Pet
	bag = s.battleBagSource(uid, st)
	forced := st.PlayerHP == 0
	if forced {
		st.markPetFainted(st.PlayerCatchTime)
	}
	picked := s.pickLivingBagPet(uid, st, bag, reqCatch)
	if picked == nil {
		s.send(c, 2407, uid, 0, buildChangePetInfo(uid, st.PlayerPetID, st.PlayerName,
			uint32(st.PlayerLevel), st.PlayerHP, st.PlayerMaxHP, st.PlayerCatchTime))
		if forced {
			log.Printf("[CMD] OK     2407 CHANGE_PET UID=%d no living pet -> fight over", uid)
			if st.isPvP() {
				oppUID := uint32(st.OpponentUID)
				opp := s.battles.get(st.OpponentUID)
				if opp != nil {
					s.finishPvP(uid, st, oppUID, opp, oppUID)
				} else {
					s.finishFight(c, uid, st, fightReasonNormal, 0)
				}
			} else {
				s.finishFight(c, uid, st, fightReasonNormal, 0)
			}
		}
		return
	}
	if uint32(picked.CatchTime) == st.PlayerCatchTime && !forced {
		// 重复选择：回当前出战信息，不推进回合
		s.send(c, 2407, uid, 0, buildChangePetInfo(uid, st.PlayerPetID, st.PlayerName,
			uint32(st.PlayerLevel), st.PlayerHP, st.PlayerMaxHP, st.PlayerCatchTime))
		log.Printf("[CMD] OK     2407 CHANGE_PET UID=%d same catch=%d", uid, reqCatch)
		return
	}

	pid, lv, name, php, patk, pdef, psa, psd, pspd := petCombatStats(picked)
	var critBonus int
	patk, pdef, psa, psd, pspd, critBonus = s.applyEnergyBallBonus(picked, patk, pdef, psa, psd, pspd)
	playerTrait := s.applyBattlePetTrait(int64(uid), picked, &critBonus)
	maxHP := uint32(php)
	hp := maxHP
	if st.IsGrandMelee {
		if picked.CurrentHP > 0 && uint32(picked.CurrentHP) < maxHP {
			hp = uint32(picked.CurrentHP)
		}
		// 换下场写回临时精灵血量
		if st.PlayerCatchTime > 0 && st.PlayerHP > 0 {
			s.setGrandMeleePlayerHP(int64(uid), st.PlayerCatchTime, st.PlayerHP)
		}
	} else {
		hp = s.recalledPetHP(int64(uid), uint32(picked.CatchTime), maxHP)
		if hp == 0 {
			hp = maxHP
		}
		if st.PlayerCatchTime > 0 && st.PlayerHP > 0 {
			s.rememberPetHP(int64(uid), st.PlayerCatchTime, st.PlayerHP)
		}
	}
	st.PlayerPetID = pid
	st.PlayerLevel = lv
	st.PlayerName = name
	st.PlayerCatchTime = uint32(picked.CatchTime)
	st.PlayerHP = hp
	st.PlayerMaxHP = maxHP
	st.PlayerAtk = patk
	st.PlayerDef = pdef
	st.PlayerSpAtk = psa
	st.PlayerSpDef = psd
	st.PlayerSpd = pspd
	st.PlayerCritBonus = critBonus
	st.PlayerTrait = playerTrait
	st.PlayerSkills = s.skillsFromPet(picked)
	st.PlayerType = s.petTypeOf(pid)
	st.PlayerDV = 0
	if picked != nil {
		st.PlayerDV = picked.DV
	}
	st.PlayerStages = [5]int8{}
	st.PlayerStatus = battleStatus{}
	st.PlayerBuff = battleBuff{}
	st.PlayerConsecSkillID = 0
	st.PlayerConsecSkillCount = 0
	st.PlayerChargeSkill = 0
	st.PlayerChargeReady = false
	st.PlayerLastTaken = 0
	st.PlayerSkillFail = false
	applyEnterPetPending(st, true)
	st.Round++
	if !st.IsGrandMelee {
		s.consumeEnergyBallOnEnter(uid, st.PlayerCatchTime)
	}
	s.battles.set(int64(uid), st)

	out := buildChangePetInfo(uid, pid, name, uint32(lv), hp, maxHP, st.PlayerCatchTime)
	s.send(c, 2407, uid, 0, out)
	log.Printf("[CMD] OK     2407 CHANGE_PET UID=%d -> pet=%d catch=%d forced=%v pvp=%v",
		uid, pid, st.PlayerCatchTime, forced, st.isPvP())

	if st.isPvP() {
		s.syncOppEnemyFromSelf(int64(uid), st, picked)
		if forced {
			// 击杀后强制换宠不占行动槽；立即通知对方
			if oc := s.clientOf(st.OpponentUID); oc != nil {
				s.send(oc, 2407, uint32(st.OpponentUID), 0, out)
			}
			return
		}
		// 主动换宠占本回合行动；对方 2407 延迟到回合结算
		switch s.pvpSubmit(uid, st, pvpActSwitch, 0, 0, st.PlayerCatchTime) {
		case pvpContinue:
			opp := s.battles.get(st.OpponentUID)
			st = s.battles.get(int64(uid))
			if opp != nil && st != nil {
				s.resolvePvPSkillRound(uid, st, uint32(st.OpponentUID), opp)
			}
		}
		return
	}

	if s.checkWildSpecialEscape(c, uid, st) {
		return
	}
	if !forced && st.EnemyHP > 0 {
		s.sendEnemyOnlyTurn(c, uid, st)
	}
}

// handleUsePetItem CMD 2406：战内用药 → UsePetItemInfo；再敌方反击。
func (s *Server) handleUsePetItem(c *Client, uid uint32, body []byte) {
	catchTime, itemID := uint32(0), uint32(0)
	if len(body) >= 8 {
		catchTime = binary.BigEndian.Uint32(body[0:4])
		itemID = binary.BigEndian.Uint32(body[4:8])
	}
	st := s.battles.get(int64(uid))
	if st == nil || !st.Active {
		log.Printf("[CMD] OK     2406 USE_PET_ITEM UID=%d (no battle)", uid)
		s.send(c, 2406, uid, 0, nil)
		return
	}
	_ = catchTime

	if st.isPvP() {
		// 参考：PvP 禁止精灵胶囊 300001–300010
		if itemID >= 300001 && itemID <= 300010 {
			s.send(c, 2406, uid, 0, nil)
			s.sendAlert(int64(uid), "对战中不能使用精灵胶囊")
			return
		}
	}

	heal := potionHealHP(itemID)
	ppRestore := potionRestorePP(itemID)
	if s.cfg.Catalog != nil {
		if h := s.cfg.Catalog.ItemHealHP(int(itemID)); h > 0 {
			heal = uint32(h)
		}
		if p := s.cfg.Catalog.ItemRestorePP(int(itemID)); p > 0 {
			ppRestore = uint32(p)
		}
	}
	if heal == 0 && ppRestore == 0 {
		log.Printf("[CMD] OK     2406 USE_PET_ITEM UID=%d item=%d (unsupported) empty", uid, itemID)
		s.send(c, 2406, uid, 0, nil)
		return
	}

	if s.cfg.Store != nil {
		if err := s.cfg.Store.ConsumeItem(int64(uid), int(itemID), 1); err != nil {
			log.Printf("[CMD] OK     2406 USE_PET_ITEM UID=%d item=%d no stock: %v", uid, itemID, err)
			s.send(c, 2406, uid, 0, nil)
			return
		}
	}

	change := int32(0)
	if heal > 0 {
		before := st.PlayerHP
		st.PlayerHP += heal
		if st.PlayerHP > st.PlayerMaxHP {
			st.PlayerHP = st.PlayerMaxHP
		}
		change = int32(st.PlayerHP - before)
	}
	if ppRestore > 0 {
		for i := range st.PlayerSkills {
			if st.PlayerSkills[i][0] == 0 {
				continue
			}
			maxPP := s.skillMaxPP(int(st.PlayerSkills[i][0]))
			st.PlayerSkills[i][1] += ppRestore
			if st.PlayerSkills[i][1] > maxPP {
				st.PlayerSkills[i][1] = maxPP
			}
		}
	}
	st.Round++
	s.battles.set(int64(uid), st)

	itemBody := buildUsePetItemInfo(uid, itemID, st.PlayerHP, change)
	s.send(c, 2406, uid, 0, itemBody)
	log.Printf("[CMD] OK     2406 USE_PET_ITEM UID=%d item=%d heal=%d pp+=%d hp=%d",
		uid, itemID, heal, ppRestore, st.PlayerHP)

	if st.isPvP() {
		s.syncOppEnemyFromSelf(int64(uid), st, nil)
		// 立即推对方 2406 同步飘字（参考 pushPvPItemUse2406ToOpponent）
		if oc := s.clientOf(st.OpponentUID); oc != nil {
			s.send(oc, 2406, uid, 0, itemBody)
		}
		switch s.pvpSubmit(uid, st, pvpActItem, 0, itemID, catchTime) {
		case pvpContinue:
			opp := s.battles.get(st.OpponentUID)
			st = s.battles.get(int64(uid))
			if opp != nil && st != nil {
				s.resolvePvPSkillRound(uid, st, uint32(st.OpponentUID), opp)
			}
		}
		return
	}
	if s.checkWildSpecialEscape(c, uid, st) {
		return
	}
	if st.EnemyHP > 0 && st.PlayerHP > 0 {
		s.sendEnemyOnlyTurn(c, uid, st)
	}
}

// handleCatchMonster CMD 2409：捕捉。
// 概率：(CatchRate/255) * hpFactor * 胶囊Bonus；Bonus≥256 必中。
// 不可捕 / 无胶囊 → catchTime=0 失败动画，再敌方反击。
func (s *Server) handleCatchMonster(c *Client, uid uint32, body []byte) {
	capsule := uint32(300001)
	if len(body) >= 4 {
		capsule = binary.BigEndian.Uint32(body[0:4])
	}
	st := s.battles.get(int64(uid))
	if st == nil || !st.Active {
		log.Printf("[CMD] OK     2409 CATCH_MONSTER UID=%d (no battle)", uid)
		s.send(c, 2409, uid, 0, buildCatchPetInfo(0, 0))
		return
	}
	if st.isPvP() {
		s.send(c, 2409, uid, 0, buildCatchPetInfo(0, 0))
		s.sendAlert(int64(uid), "对战中不能捕捉")
		return
	}

	failCatch := func(reason string) {
		s.send(c, 2409, uid, 0, buildCatchPetInfo(0, 0))
		st.Round++
		s.battles.set(int64(uid), st)
		log.Printf("[CMD] OK     2409 CATCH_MONSTER UID=%d capsule=%d fail enemy=%d (%s)", uid, capsule, st.EnemyID, reason)
		if s.checkWildSpecialEscape(c, uid, st) {
			return
		}
		if st.EnemyHP > 0 {
			s.sendEnemyOnlyTurn(c, uid, st)
		}
	}

	if !st.EnemyCatchable {
		failCatch("uncatchable")
		return
	}
	if st.EnemyMaxHP == 0 || st.EnemyHP == 0 {
		failCatch("enemy dead")
		return
	}

	bonus := float64(0)
	if s.cfg.Catalog != nil {
		bonus = s.cfg.Catalog.ItemCatchBonusOf(int(capsule))
	}
	if bonus <= 0 {
		// 表无 Bonus 时：常见胶囊兜底；未知道具拒捕
		switch capsule {
		case 300001:
			bonus = 1
		case 300002:
			bonus = 1.5
		case 300003:
			bonus = 2
		case 300004:
			bonus = 3
		case 300005:
			bonus = 4
		case 300006, 300007, 300009, 300010:
			bonus = 256
		default:
			s.send(c, 2409, uid, 0, buildCatchPetInfo(0, 0))
			s.sendAlert(int64(uid), "不是捕捉胶囊")
			return
		}
	}

	if s.cfg.Store != nil {
		if err := s.cfg.Store.ConsumeItem(int64(uid), int(capsule), 1); err != nil {
			s.send(c, 2409, uid, 0, buildCatchPetInfo(0, 0))
			s.sendAlert(int64(uid), "胶囊不足")
			return
		}
	}

	caught := false
	catchRate := 45
	hpFactor := 0.3 + 0.7*(1.0-float64(st.EnemyHP)/float64(st.EnemyMaxHP))
	if bonus >= 256 {
		caught = true
	} else {
		if s.cfg.Catalog != nil {
			catchRate = s.cfg.Catalog.CatchRateOf(st.EnemyID)
		}
		ctrl := enemyCatchStatusBoost(st)
		prob := calcCatchProbability(catchRate, st.EnemyHP, st.EnemyMaxHP, bonus, ctrl)
		roll := rand.Float64()
		caught = roll < prob
		log.Printf("[2409] catch roll UID=%d pet=%d rate=%d hpF=%.2f bonus=%.2f ctrl=%v prob=%.3f roll=%.3f -> %v",
			uid, st.EnemyID, catchRate, hpFactor, bonus, ctrl, prob, roll, caught)
	}

	if caught {
		catchTm := uint32(0)
		if s.cfg.Store != nil {
			ct, err := s.grantNewPet(int64(uid), st.EnemyID, st.EnemyLevel)
			if err != nil {
				log.Printf("[2409] grant pet fail UID=%d: %v", uid, err)
				failCatch("grant fail")
				return
			}
			catchTm = uint32(ct)
		} else {
			catchTm = uint32(time.Now().Unix())
		}
		s.send(c, 2409, uid, 0, buildCatchPetInfo(catchTm, uint32(st.EnemyID)))
		log.Printf("[CMD] OK     2409 CATCH_MONSTER UID=%d success pet=%d catch=%d", uid, st.EnemyID, catchTm)
		s.finishFight(c, uid, st, fightReasonNormal, uid)
		return
	}

	failCatch("miss")
}

// calcCatchProbability 捕捉成功率：baseRate * hpFactor * bonus [* statusMod]，封顶 0.99。
// statusControlled：敌方处于睡眠/麻痹/恐惧/疲惫/冰冻等控制时 ×1.5（尼尔号游玩须知）。
func calcCatchProbability(catchRate int, hp, maxHP uint32, bonus float64, statusControlled bool) float64 {
	if bonus >= 256 {
		return 1
	}
	if maxHP == 0 || bonus <= 0 || catchRate <= 0 {
		return 0
	}
	if catchRate > 255 {
		catchRate = 255
	}
	hpFactor := 0.3 + 0.7*(1.0-float64(hp)/float64(maxHP))
	prob := (float64(catchRate) / 255.0) * hpFactor * bonus
	if statusControlled {
		prob *= 1.5
	}
	if prob > 0.99 {
		prob = 0.99
	}
	if prob < 0 {
		return 0
	}
	return prob
}

func enemyCatchStatusBoost(st *BattleState) bool {
	if st == nil {
		return false
	}
	s := st.EnemyStatus
	return s.Sleep || s.Para || s.Fear || s.Tired || s.Freeze
}

// sendEnemyOnlyTurn 仅敌方攻击的半回合（换宠/用药/捕捉失败后）。
func (s *Server) sendEnemyOnlyTurn(c *Client, uid uint32, st *BattleState) {
	if st == nil || !st.Active {
		return
	}
	enemySkill := s.pickEnemyBattleSkill(st)
	if enemySkill == 0 {
		enemySkill = 10001
	}
	if !enemyHasInfinitePP(st) {
		decSkillPP(st.EnemySkills, enemySkill)
	}
	if consumeSkipStatus(&st.EnemyStatus) {
		enemyAv := buildAttackValueFromState(0, 0, 0, 0, 0, int32(st.EnemyHP), st.EnemyMaxHP, 0, 0, 0, st, false, nil)
		idleAv := buildAttackValueFromState(uid, 0, 0, 0, 0, int32(st.PlayerHP), st.PlayerMaxHP, 0, 0, 0, st, true, st.PlayerSkills)
		out := append(enemyAv, idleAv...)
		s.send(c, 2505, uid, 0, out)
		log.Printf("[CMD] OK     2505 NOTE_USE_SKILL UID=%d (enemy status skip)", uid)
		s.battles.set(int64(uid), st)
		return
	}
	if !s.checkSkillHitTrait(enemySkill, 0, 0, 0, st.PlayerTrait) {
		enemyAv := buildAttackValueFromState(0, enemySkill, 0, 0, 0, int32(st.EnemyHP), st.EnemyMaxHP, 0, 0, 0, st, false, nil)
		idleAv := buildAttackValueFromState(uid, 0, 0, 0, 0, int32(st.PlayerHP), st.PlayerMaxHP, 0, 0, 0, st, true, st.PlayerSkills)
		out := append(enemyAv, idleAv...)
		tickStatusDamage(st)
		s.send(c, 2505, uid, 0, out)
		log.Printf("[CMD] OK     2505 NOTE_USE_SKILL UID=%d (enemy miss) skill=%d", uid, enemySkill)
		s.battles.set(int64(uid), st)
		return
	}
	eDef := s.skillDef(int(enemySkill))
	hits := sideEffectHitCount(eDef)
	dmg := s.damageWithSkill(enemySkill, st.EnemyLevel,
		st.stagedAtk(false), st.stagedDef(true), st.stagedSpAtk(false), st.stagedSpDef(true),
		st.EnemyType, st.PlayerType)
	dmg *= uint32(hits)
	// 28/29/93 粉伤不并入 lostHP
	dmg = applyLeaveOneHP(eDef, st.PlayerHP, dmg)
	if dmg > 0 {
		dmg = applyTraitIncomingDamage(st.PlayerTrait, st.PlayerHP, dmg)
	}
	dmg = applyBossHalfHPOneShot(st, dmg, true)
	foeHPBefore := st.PlayerHP
	pink := sideEffectPinkDamage(eDef, foeHPBefore)
	actual := applyDamage(&st.PlayerHP, dmg)
	applyPinkDamage(&st.PlayerHP, pink)
	s.applySkillSideEffects(st, enemySkill, actual, false, true)
	applyPvEPlayerTraitOnHit(st, enemySkill, actual, eDef)
	tickStatusDamage(st)

	enemyAv := buildAttackValueFromState(0, enemySkill, uint32(hits), dmg, 0, int32(st.EnemyHP), st.EnemyMaxHP, 0, 0, 0, st, false, nil)
	idleAv := buildAttackValueFromState(uid, 0, 0, 0, 0, int32(st.PlayerHP), st.PlayerMaxHP, 0, 0, 0, st, true, st.PlayerSkills)
	out := append(enemyAv, idleAv...)
	s.send(c, 2505, uid, 0, out)
	log.Printf("[CMD] OK     2505 NOTE_USE_SKILL UID=%d (enemy only) dmg=%d actual=%d hp=%d", uid, dmg, actual, st.PlayerHP)

	if st.PlayerHP == 0 {
		st.markPetFainted(st.PlayerCatchTime)
		// 还有其它存活精灵则等客户端 2407；否则战败
		if s.hasOtherLivingPet(uid, st) {
			s.battles.set(int64(uid), st)
			return
		}
		s.finishFight(c, uid, st, fightReasonNormal, 0)
		return
	}
	s.battles.set(int64(uid), st)
}

func (s *Server) hasOtherLivingPet(uid uint32, st *BattleState) bool {
	if st == nil || !st.allowsPetSwitch() {
		return false
	}
	bag := s.battleBagSource(uid, st)
	for i := range bag {
		ct := uint32(bag[i].CatchTime)
		if ct == 0 || ct == st.PlayerCatchTime || st.isPetFainted(ct) {
			continue
		}
		return true
	}
	return false
}

// pickLivingBagPet 选一只本场未倒地的背包精灵（优先 reqCatch）。
func (s *Server) pickLivingBagPet(uid uint32, st *BattleState, bag []store.Pet, reqCatch uint32) *store.Pet {
	if st == nil || len(bag) == 0 {
		return nil
	}
	_ = uid
	try := func(ct uint32) *store.Pet {
		if ct == 0 || st.isPetFainted(ct) {
			return nil
		}
		// 主动换宠：不能选当前仍存活的同一只（由上层 same-catch 处理）；强制换宠时当前已 mark 倒地
		if ct == st.PlayerCatchTime && st.PlayerHP > 0 && !st.isPetFainted(ct) {
			return nil
		}
		for i := range bag {
			if uint32(bag[i].CatchTime) == ct {
				return &bag[i]
			}
		}
		return nil
	}
	if p := try(reqCatch); p != nil {
		return p
	}
	for i := range bag {
		if p := try(uint32(bag[i].CatchTime)); p != nil {
			return p
		}
	}
	return nil
}

func applyDamage(hp *uint32, dmg uint32) (lost uint32) {
	if dmg >= *hp {
		lost = *hp
		*hp = 0
		return lost
	}
	*hp -= dmg
	return dmg
}

func potionHealHP(itemID uint32) uint32 {
	switch itemID {
	case 300011:
		return 20
	case 300012:
		return 50
	case 300013:
		return 100
	case 300014:
		return 150
	case 300015, 300076, 300077:
		return 200
	case 300020:
		return 3000
	case 300154:
		return 200
	case 300155:
		return 150
	case 300156:
		return 100
	default:
		return 0
	}
}

func potionRestorePP(itemID uint32) uint32 {
	switch itemID {
	case 300016:
		return 5
	case 300017, 300023, 300073:
		return 10
	case 300018, 300074:
		return 20
	case 300019:
		return 40
	default:
		return 0
	}
}
