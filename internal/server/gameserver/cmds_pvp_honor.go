package gameserver

import (
	"log"
	"strconv"
)

// 精灵王之战荣誉（攻略八）：
// 1v1：每天前 2 局各 +4（共 8，输赢皆可）
// 6v6：每天第一局 +9；之后每胜 +1，直至周上限
const (
	pvp1v1DailyCap     = 2
	pvp1v1HonorEach    = 4
	pvp6v6FirstHonor   = 9
	pvp6v6WinHonor     = 1
	pvp6v6WeekHonorCap = 40
)

func (s *Server) grantPvPHonor(uid uint32, st *BattleState, won bool) {
	if st == nil || !st.isPvP() {
		return
	}
	add := 0
	switch st.PvPMode {
	case pvpModeSingle:
		n := s.bumpDaily(int64(uid), "pvp1v1")
		if n <= pvp1v1DailyCap {
			add = pvp1v1HonorEach
		}
	default: // 多精灵按 6v6 处理
		if s.tryMarkDaily(int64(uid), "pvp6v6First") {
			add = pvp6v6FirstHonor
		} else if won {
			w := s.weeklyCount(int64(uid), "pvp6v6Honor")
			if w < pvp6v6WeekHonorCap {
				s.bumpWeekly(int64(uid), "pvp6v6Honor")
				add = pvp6v6WinHonor
			}
		}
	}
	if add <= 0 {
		return
	}
	s.addHonor(int64(uid), add)
	s.sendAlert(int64(uid), "对战荣誉+"+strconv.Itoa(add))
	log.Printf("[pvp] honor UID=%d +%d mode=%d won=%v", uid, add, st.PvPMode, won)
}
