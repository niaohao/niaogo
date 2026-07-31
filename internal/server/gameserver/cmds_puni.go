package gameserver

import (
	"fmt"
	"log"
)

// 谱尼封印日限键；region 1..7 封印、8 真身。对照 MapProcess_514 dailyResArr[40..47]。
const puniDailyKeyFmt = "puniSeal_%d"

// 服务端错误码 → ParseSocketError：11025→ERROR_11027（需先破前一封印），11027→ERROR_11028（今日已封）。
const (
	errPuniNeedPrevSeal  int32 = 11025
	errPuniDailyClosed   int32 = 11027
)

func puniDailyKey(region uint32) string {
	return fmt.Sprintf(puniDailyKeyFmt, region)
}

func puniMapID(mapID int) int {
	if alias, ok := bossMapAlias[mapID]; ok {
		mapID = alias
	}
	return mapID
}

func isPuniChallengeMap(mapID int) bool {
	return puniMapID(mapID) == 514
}

// maxPuniLvOf 已连续解除的封印数（1..8）；对照登录包 maxPuniLv。
func (s *Server) maxPuniLvOf(uid int64) uint32 {
	if s.cfg.Store == nil {
		return 0
	}
	var lv uint32
	for r := uint32(1); r <= 8; r++ {
		ok, err := s.cfg.Store.HasDefeatedSPT(uid, puniSealDefeatBase+int(r))
		if err != nil || !ok {
			break
		}
		lv = r
	}
	return lv
}

// fillPuniDailyRes 写入 dailyResArr[40..47]（今日已挑战的封印/真身）。
func (s *Server) fillPuniDailyRes(uid int64, arr []byte) {
	if len(arr) < 48 {
		return
	}
	for r := uint32(1); r <= 8; r++ {
		if s.dailyCount(uid, puniDailyKey(r)) > 0 {
			arr[40+int(r)-1] = 1
		}
	}
}

// gatePuniChallenge 谱尼顺序封印 + 日限；失败时回 2411 带错误码且不开战。
func (s *Server) gatePuniChallenge(c *Client, uid uint32, mapID int, region uint32) bool {
	if !isPuniChallengeMap(mapID) {
		return true
	}
	if region < 1 || region > 8 {
		return true
	}
	u := int64(uid)
	if s.dailyCount(u, puniDailyKey(region)) > 0 {
		// 真身可无限挑战（对齐尼尔号/参考服服务端）
		if region != puniRegionTrue {
			s.send(c, 2411, uid, errPuniDailyClosed, nil)
			log.Printf("[CMD] DENY  2411 CHALLENGE_BOSS UID=%d puni region=%d daily", uid, region)
			return false
		}
	}
	need := region - 1 // 真身 region=8 需 maxPuniLv>=7
	if need > 0 && s.maxPuniLvOf(u) < need {
		s.send(c, 2411, uid, errPuniNeedPrevSeal, nil)
		log.Printf("[CMD] DENY  2411 CHALLENGE_BOSS UID=%d puni region=%d needPrev=%d", uid, region, need)
		return false
	}
	return true
}

func (s *Server) markPuniDailyChallenge(uid uint32, mapID int, region uint32) {
	if !isPuniChallengeMap(mapID) || region < 1 || region > 8 {
		return
	}
	if region == puniRegionTrue {
		return // 真身不占日限
	}
	s.bumpDaily(int64(uid), puniDailyKey(region))
}
