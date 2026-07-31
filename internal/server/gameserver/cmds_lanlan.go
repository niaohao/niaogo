package gameserver

import (
	"log"
	"strconv"
)

// 岚岚日常 Boss（地图 108，param2=10/11/12 → 三挡）。
// 攻略：①2w经验/豆+2荣誉+学习力双倍仪 ②4w…+微型双倍 ③10w…+微型双倍×2+扭蛋币×8
const (
	lanlanMapID   = 108
	lanlanPetID   = 3110
	lanlanEasy    = 10
	lanlanNormal  = 11
	lanlanHard    = 12

	lanlanItemDualEV   = 300035 // 学习力双倍仪
	lanlanItemMicroExp = 300101 // 微型双倍经验加速器
	lanlanItemGacha    = 400501 // 扭蛋币
)

func resolveLanlanBoss(mapID int, param2 uint32) (petID, level int, name string, honor int, ok bool) {
	if mapID != lanlanMapID {
		return 0, 0, "", 0, false
	}
	switch param2 {
	case lanlanEasy:
		return lanlanPetID, 80, "岚岚", 2, true
	case lanlanNormal:
		return lanlanPetID, 90, "岚岚", 4, true
	case lanlanHard:
		return lanlanPetID, 100, "岚岚", 6, true
	}
	return 0, 0, "", 0, false
}

func (s *Server) grantLanlanHonorReward(c *Client, uid uint32, st *BattleState) {
	if st == nil || st.EnemyID != lanlanPetID || s.cfg.Store == nil {
		return
	}
	var honor, exp, coins, dualEV, microExp, gacha int
	switch st.BossRegion {
	case lanlanEasy:
		honor, exp, coins, dualEV = 2, 20000, 20000, 1
	case lanlanNormal:
		honor, exp, coins, dualEV, microExp = 4, 40000, 40000, 1, 1
	case lanlanHard:
		honor, exp, coins, dualEV, microExp, gacha = 6, 100000, 100000, 1, 2, 8
	default:
		return
	}
	key := "lanlanHonor"
	if !s.tryMarkDaily(int64(uid), key) {
		return
	}
	s.addHonor(int64(uid), honor)
	_, _ = s.cfg.Store.AddExpPool(int64(uid), exp)
	_ = s.cfg.Store.AddCoins(int64(uid), coins)
	if dualEV > 0 {
		_ = s.cfg.Store.AddItem(int64(uid), lanlanItemDualEV, dualEV)
	}
	if microExp > 0 {
		_ = s.cfg.Store.AddItem(int64(uid), lanlanItemMicroExp, microExp)
	}
	if gacha > 0 {
		_ = s.cfg.Store.AddItem(int64(uid), lanlanItemGacha, gacha)
	}
	s.sendAlert(int64(uid), "岚岚挑战奖励：荣誉+"+strconv.Itoa(honor)+
		" 经验+"+strconv.Itoa(exp)+" 豆+"+strconv.Itoa(coins))
	log.Printf("[lanlan] reward UID=%d honor=%d exp=%d coins=%d region=%d", uid, honor, exp, coins, st.BossRegion)
}
