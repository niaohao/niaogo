package gameserver

import (
	"log"
	"strconv"
)

// 勇者之塔：每日前 2 次胜利合计 10w 经验 + 1w 豆 + 20 荣誉（每次各半）。
const (
	braveDailyWinCap     = 2
	braveDailyExpEach    = 50000
	braveDailyCoinsEach  = 5000
	braveDailyHonorEach  = 10
	trainCampFlashNeilID = 310
)

func (s *Server) grantBraveTowerDailyReward(c *Client, uid uint32, st *BattleState) {
	if st == nil || st.FightKind != fightKindBrave || s.cfg.Store == nil {
		return
	}
	n := s.bumpDaily(int64(uid), "braveWin")
	if n > braveDailyWinCap {
		return
	}
	_, _ = s.cfg.Store.AddExpPool(int64(uid), braveDailyExpEach)
	_ = s.cfg.Store.AddCoins(int64(uid), braveDailyCoinsEach)
	s.addHonor(int64(uid), braveDailyHonorEach)
	s.sendAlert(int64(uid), "勇者之塔日常奖励：经验+"+strconv.Itoa(braveDailyExpEach)+
		" 豆+"+strconv.Itoa(braveDailyCoinsEach)+" 荣誉+"+strconv.Itoa(braveDailyHonorEach))
	log.Printf("[tower] daily UID=%d win#%d", uid, n)
}

// grantTrainCampSetBonus 三主宠日常 401–403 全部完成后发闪光尼尔（合计奖励已拆到各任务）。
func (s *Server) grantTrainCampSetBonus(uid int64, justCompleted int) {
	if s.cfg.Store == nil || justCompleted < 401 || justCompleted > 403 {
		return
	}
	for _, id := range []int{401, 402, 403} {
		if id == justCompleted {
			continue
		}
		if !s.taskAlreadyComplete(uid, id) {
			return
		}
	}
	if !s.tryMarkDaily(uid, "trainCampFlashNeil") {
		return
	}
	if _, err := s.grantNewPet(uid, trainCampFlashNeilID, 1); err != nil {
		log.Printf("[task] flash neil uid=%d: %v", uid, err)
		return
	}
	s.addHonor(uid, 9)
	s.sendAlert(uid, "训练营全部完成，获得闪光尼尔×1、荣誉+9")
}
