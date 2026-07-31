package gameserver

import (
	"encoding/binary"
	"log"
	"strconv"
)

// 匹配日常经验（攻略九）：
// 精灵王之战：输赢皆 4w，日限 2
// 精灵大乱斗：输赢皆 5w，日限 2
const (
	petKingDailyExpCap   = 2
	petKingDailyExpEach  = 40000
	petKingDailyExpKey   = "petKingExp"

	grandMeleeDailyExpCap  = 2
	grandMeleeDailyExpEach = 50000
	grandMeleeDailyExpKey  = "grandMeleeExp"
)

// grantMatchDailyExp 王战/大乱斗结算发积累经验（输赢皆可，有日限）。
func (s *Server) grantMatchDailyExp(c *Client, uid uint32, st *BattleState) {
	if st == nil || s.cfg.Store == nil {
		return
	}
	kind := st.DailyExpKind
	if kind == dailyExpNone && st.isPvP() && st.PvPMode == pvpModeSingle {
		kind = dailyExpPetKing
	}
	if kind == dailyExpNone && st.IsGrandMelee {
		kind = dailyExpGrandMelee
	}
	var key string
	var cap, each int
	var label string
	switch kind {
	case dailyExpPetKing:
		key, cap, each, label = petKingDailyExpKey, petKingDailyExpCap, petKingDailyExpEach, "精灵王之战"
	case dailyExpGrandMelee:
		key, cap, each, label = grandMeleeDailyExpKey, grandMeleeDailyExpCap, grandMeleeDailyExpEach, "精灵大乱斗"
	default:
		return
	}
	n := s.bumpDaily(int64(uid), key)
	if n > cap {
		return
	}
	_, _ = s.cfg.Store.AddExpPool(int64(uid), each)
	s.pushPetWarExpNotice(c, uid, each)
	s.sendAlert(int64(uid), label+"日常经验+"+strconv.Itoa(each)+"（今日"+strconv.Itoa(n)+"/"+strconv.Itoa(cap)+"）")
	log.Printf("[match-exp] UID=%d kind=%d +%d day=%d/%d", uid, kind, each, n, cap)
}

// pushPetWarExpNotice CMD 2509：客户端弹「获得 N 点积累经验」。
func (s *Server) pushPetWarExpNotice(c *Client, uid uint32, exp int) {
	if c == nil || exp <= 0 {
		return
	}
	body := make([]byte, 4)
	binary.BigEndian.PutUint32(body, uint32(exp))
	s.send(c, 2509, uid, 0, body)
}
