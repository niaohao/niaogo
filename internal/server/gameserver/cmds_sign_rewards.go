package gameserver

import (
	"log"
	"strconv"
)

// 超 No 签到 1–30 日奖励表（语雀《尼尔号游玩须知》）。
// 虚拟奖励：1=赛尔豆 3=积累经验；其余为道具/精灵 ID（精灵用 grant 特判）。
const (
	signRewardPetNeil      = 77
	signRewardPetKalu      = 315 // 卡鲁耶克
	signItemTraitChip      = 300063
	signItemFastHatch      = 400082
	signItemTripleExp      = 300051
	signItemFormRestore    = 300153
	signItemFormLock       = 300152
)

func nonoVipSignDayRewards(day int) [][2]uint32 {
	switch day {
	case 1:
		return [][2]uint32{{1, 5000}, {3, 10000}, {signItemTraitChip, 1}}
	case 2:
		return [][2]uint32{{1, 10000}, {3, 20000}}
	case 3, 6, 9, 15, 18, 21, 24, 27:
		return [][2]uint32{{signRewardPetNeil, 1}, {1, 5000}, {3, 10000}, {signItemFastHatch, 1}}
	case 4:
		return [][2]uint32{{1, 15000}, {3, 30000}}
	case 5, 10, 22:
		return [][2]uint32{{signItemTripleExp, 1}, {1, 5000}, {3, 10000}}
	case 7:
		return [][2]uint32{{1, 20000}, {3, 40000}}
	case 8:
		return [][2]uint32{{1, 25000}, {3, 50000}}
	case 11:
		return [][2]uint32{{1, 30000}, {3, 60000}}
	case 12:
		return [][2]uint32{{signRewardPetNeil, 1}, {1, 5000}, {3, 10000}, {signItemFastHatch, 1}}
	case 13:
		return [][2]uint32{{1, 35000}, {3, 70000}}
	case 14:
		return [][2]uint32{{signRewardPetKalu, 1}, {signItemTraitChip, 1}, {1, 5000}, {3, 10000}}
	case 16:
		return [][2]uint32{{1, 40000}, {3, 80000}}
	case 17, 19:
		return [][2]uint32{{1, 45000}, {3, 90000}}
	case 20:
		return [][2]uint32{{signItemFormRestore, 1}, {1, 5000}, {3, 10000}}
	case 23:
		return [][2]uint32{{1, 50000}, {3, 100000}}
	case 25:
		return [][2]uint32{{signItemFormLock, 1}, {signItemTraitChip, 1}, {1, 5000}, {3, 10000}}
	case 26:
		return [][2]uint32{{1, 55000}, {3, 110000}}
	case 28:
		return [][2]uint32{{1, 60000}, {3, 120000}}
	case 29:
		return [][2]uint32{{1, 65000}, {3, 130000}}
	case 30:
		return [][2]uint32{{1, 100000}, {3, 200000}, {signItemTripleExp, 1}}
	default:
		return [][2]uint32{{1, 5000}, {3, 10000}}
	}
}

func (s *Server) grantNonoVipSignRewards(uid int64, day int) {
	if s.cfg.Store == nil || day < 1 {
		return
	}
	pairs := nonoVipSignDayRewards(day)
	var msgParts []string
	for _, p := range pairs {
		id, cnt := int(p[0]), int(p[1])
		if id <= 0 || cnt <= 0 {
			continue
		}
		switch id {
		case taskRewardCoins:
			_ = s.cfg.Store.AddCoins(uid, cnt)
			msgParts = append(msgParts, "赛尔豆×"+strconv.Itoa(cnt))
		case taskRewardExpPool:
			_, _ = s.cfg.Store.AddExpPool(uid, cnt)
			msgParts = append(msgParts, "经验×"+strconv.Itoa(cnt))
		case signRewardPetNeil, signRewardPetKalu:
			for i := 0; i < cnt; i++ {
				if _, err := s.grantNewPet(uid, id, 1); err != nil {
					log.Printf("[sign] grant pet uid=%d pet=%d: %v", uid, id, err)
				}
			}
			name := "精灵"
			if s.cfg.Catalog != nil {
				if n := s.cfg.Catalog.PetNameOf(id); n != "" {
					name = n
				}
			}
			msgParts = append(msgParts, name+"×"+strconv.Itoa(cnt))
		default:
			_ = s.cfg.Store.AddItem(uid, id, cnt)
			msgParts = append(msgParts, "道具"+strconv.Itoa(id)+"×"+strconv.Itoa(cnt))
		}
	}
	if len(msgParts) > 0 {
		s.sendAlert(uid, "签到成功："+msgParts[0])
		for i := 1; i < len(msgParts) && i < 4; i++ {
			s.sendAlert(uid, "获得 "+msgParts[i])
		}
	}
}
