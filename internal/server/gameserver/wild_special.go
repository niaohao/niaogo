package gameserver

import (
	"log"
	"math/rand"
	"time"
)

// 尼尔号特殊野生遭遇（对齐语雀须知：尼尔/尼奥/洛森）。
const (
	wildPetNeil      = 77
	wildPetFlashNeil = 310
	wildPetNiao      = 416
	wildPetLuosen    = 486
	wildPetAifeidesi = 1459
	wildPetDaer      = 122 // 达尔：不触发尼尔/尼奥替换
)

// 遭遇概率：全天约 5%；20:00–23:59 尼尔/尼奥平分，其余时段仅尼尔。
const (
	wildSpecialChancePercent = 5
	niaoHourStart            = 20
	niaoHourEnd              = 23
)

// tryReplaceWildSpecial 小形态普通野怪开战前，有概率替换为尼尔/尼奥。
func (s *Server) tryReplaceWildSpecial(enemyID, enemyLv int) (id, lv int, replaced bool) {
	id, lv = enemyID, enemyLv
	if enemyID == wildPetNeil || enemyID == wildPetNiao || enemyID == wildPetLuosen {
		return
	}
	if enemyID == wildPetDaer {
		return
	}
	if !s.isWildBaseForm(enemyID) || s.isRareWildPet(enemyID) {
		return
	}
	if rand.Intn(100) >= wildSpecialChancePercent {
		return
	}
	hour := chinaNow(time.Now()).Hour()
	inNiaoWindow := hour >= niaoHourStart && hour <= niaoHourEnd
	if inNiaoWindow && rand.Intn(2) == 0 {
		id = wildPetNiao
		lv = 17 + rand.Intn(2)
	} else {
		id = wildPetNeil
		lv = 16 + rand.Intn(2)
	}
	replaced = true
	return
}

func (s *Server) isWildBaseForm(petID int) bool {
	if petID <= 0 {
		return false
	}
	if s.cfg.Catalog == nil {
		return true
	}
	d := s.cfg.Catalog.PetBase(petID)
	if d == nil {
		return true
	}
	return d.EvolvesFrom <= 0
}

func (s *Server) isRareWildPet(petID int) bool {
	if s.cfg.Catalog == nil {
		return false
	}
	d := s.cfg.Catalog.PetBase(petID)
	return d != nil && d.IsRareMon
}

func (s *Server) evolutionRoot(petID int) int {
	if petID <= 0 {
		return 0
	}
	if s.cfg.Catalog != nil {
		return s.cfg.Catalog.EvolutionBaseForm(petID)
	}
	return petID
}

// blocksNiaoEscape：尼尔系（含闪光尼尔链）或艾菲德斯站场可挡尼奥逃跑。
func (s *Server) blocksNiaoEscape(petID int) bool {
	if petID == wildPetAifeidesi || petID == wildPetNeil || petID == wildPetFlashNeil {
		return true
	}
	root := s.evolutionRoot(petID)
	return root == wildPetNeil || root == wildPetFlashNeil
}

// blocksNeilEscape：尼奥系或艾菲德斯站场可挡尼尔逃跑。
func (s *Server) blocksNeilEscape(petID int) bool {
	if petID == wildPetAifeidesi || petID == wildPetNiao {
		return true
	}
	return s.evolutionRoot(petID) == wildPetNiao
}

// checkWildSpecialEscape 回合推进后：尼奥≥2 / 尼尔≥6(无控制) / 洛森≥4(无控制) 逃跑。
// 返回 true 表示已结束战斗。
func (s *Server) checkWildSpecialEscape(c *Client, uid uint32, st *BattleState) bool {
	if st == nil || !st.Active || !st.IsWildMonster || st.isPvP() {
		return false
	}
	switch st.EnemyID {
	case wildPetNiao:
		if s.blocksNiaoEscape(st.PlayerPetID) {
			st.HasSeenEscapeBlock = true
		}
		if !st.HasSeenEscapeBlock && st.Round >= 2 {
			s.finishWildSpecialEscape(c, uid, st, "尼奥")
			return true
		}
	case wildPetNeil:
		if s.blocksNeilEscape(st.PlayerPetID) {
			st.HasSeenEscapeBlock = true
		}
		if !st.HasSeenEscapeBlock && st.Round >= 6 && !enemyCatchStatusBoost(st) {
			s.finishWildSpecialEscape(c, uid, st, "尼尔")
			return true
		}
	case wildPetLuosen:
		if st.Round >= 4 && !enemyCatchStatusBoost(st) {
			s.finishWildSpecialEscape(c, uid, st, "洛森")
			return true
		}
	}
	return false
}

func (s *Server) finishWildSpecialEscape(c *Client, uid uint32, st *BattleState, name string) {
	if st == nil {
		return
	}
	log.Printf("[wild] %s escaped UID=%d round=%d", name, uid, st.Round)
	mapID := st.MapID
	if st.PlayerCatchTime > 0 {
		if st.PlayerHP == 0 {
			s.forgetPetHP(int64(uid), st.PlayerCatchTime)
		} else {
			s.rememberPetHP(int64(uid), st.PlayerCatchTime, st.PlayerHP)
		}
	}
	s.battles.clear(int64(uid))
	bt := s.boostTimesOf(int64(uid))
	s.send(c, 2506, uid, 0, buildFightOverInfoTimes(fightReasonEscape, 0,
		uint32(max0(bt.TwoTimes)), uint32(max0(bt.ThreeTimes)),
		uint32(max0(bt.AutoFightTimes)), uint32(max0(bt.EnergyTimes)),
		uint32(max0(bt.LearnTimes))))
	s.refreshMapOgresAfterFight(c, uid, mapID)
	s.sendAlert(int64(uid), name+"逃走了……")
}
