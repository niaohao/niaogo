package gameserver

import (
	"log"
	"strconv"
)

// 排位赛「暂定」日常（攻略九）：挂在王战 1v1 结算上。
// 前 5 场不论输赢发豆；当日第 1 胜扭蛋币；第 3 胜特性重组剂。
const (
	rankDailyMatchCap   = 5
	rankDailyCoinsEach  = 5000
	rankDailyMatchKey   = "rankMatch"
	rankDailyWinKey     = "rankWin"
	rankGachaOnFirstWin = 2
	rankGachaItemID     = 400501
	rankTraitOnThirdWin = 300054 // 特性重组药剂
)

func (s *Server) grantRankDailyReward(uid uint32, st *BattleState, won bool) {
	if st == nil || !st.isPvP() || st.PvPMode != pvpModeSingle || s.cfg.Store == nil {
		return
	}
	n := s.bumpDaily(int64(uid), rankDailyMatchKey)
	if n <= rankDailyMatchCap {
		_ = s.cfg.Store.AddCoins(int64(uid), rankDailyCoinsEach)
		s.sendAlert(int64(uid), "排位日常：赛尔豆+"+strconv.Itoa(rankDailyCoinsEach)+
			"（今日"+strconv.Itoa(n)+"/"+strconv.Itoa(rankDailyMatchCap)+"）")
	}
	if !won {
		return
	}
	wins := s.bumpDaily(int64(uid), rankDailyWinKey)
	switch wins {
	case 1:
		_ = s.cfg.Store.AddItem(int64(uid), rankGachaItemID, rankGachaOnFirstWin)
		s.sendAlert(int64(uid), "排位首胜：扭蛋币×"+strconv.Itoa(rankGachaOnFirstWin))
	case 3:
		_ = s.cfg.Store.AddItem(int64(uid), rankTraitOnThirdWin, 1)
		s.sendAlert(int64(uid), "排位第3胜：特性重组药剂×1")
	}
	log.Printf("[rank-daily] UID=%d match=%d win=%d won=%v", uid, n, wins, won)
}
