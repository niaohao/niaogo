package gameserver

import (
	"log"
	"math/rand"
	"strconv"
)

// 周常光/暗螳螂（地图 102）：共享周次数；奖励材料 10–13 + 20w 经验/豆 + 三倍经验加速器。
const (
	mantisMapID           = 102
	mantisPetLight        = 124 // 英卡洛斯
	mantisPetDark         = 125 // 萨格罗斯
	mantisItemLight       = 403022
	mantisItemDark        = 403023
	mantisWeekKey         = "mantisWeek"
	mantisExpReward       = 200000
	mantisCoinsReward     = 200000
	mantisTripleExpItem   = 300051
)

func resolveMantisBoss(mapID int, param2 uint32) (petID, level int, name string, ok bool) {
	if mapID != mantisMapID {
		return 0, 0, "", false
	}
	switch param2 {
	case 0:
		return mantisPetLight, 100, "英卡洛斯", true
	case 1:
		return mantisPetDark, 100, "萨格罗斯", true
	}
	return 0, 0, "", false
}

func (s *Server) grantMantisWeeklyReward(c *Client, uid uint32, st *BattleState) {
	if st == nil || s.cfg.Store == nil {
		return
	}
	if st.MapID != mantisMapID && st.EnemyID != mantisPetLight && st.EnemyID != mantisPetDark {
		return
	}
	if st.EnemyID != mantisPetLight && st.EnemyID != mantisPetDark {
		return
	}
	if !s.tryMarkWeekly(int64(uid), mantisWeekKey) {
		s.sendAlert(int64(uid), "本周螳螂挑战奖励已领取（光/暗共享）")
		return
	}
	itemID := mantisItemLight
	if st.EnemyID == mantisPetDark {
		itemID = mantisItemDark
	}
	cnt := 10 + rand.Intn(4) // 10–13
	_ = s.cfg.Store.AddItem(int64(uid), itemID, cnt)
	_, _ = s.cfg.Store.AddExpPool(int64(uid), mantisExpReward)
	_ = s.cfg.Store.AddCoins(int64(uid), mantisCoinsReward)
	_ = s.cfg.Store.AddItem(int64(uid), mantisTripleExpItem, 1)
	s.sendAlert(int64(uid), "周常螳螂奖励：材料×"+strconv.Itoa(cnt)+
		" 经验+20万 豆+20万 三倍经验×1")
	log.Printf("[mantis] weekly UID=%d enemy=%d item=%d x%d", uid, st.EnemyID, itemID, cnt)
}
