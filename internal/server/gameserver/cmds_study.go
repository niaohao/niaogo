package gameserver

import (
	"encoding/binary"
	"log"

	"niaohao/server/internal/cmdname"
	"niaohao/server/internal/store"
)

// 学习力双倍仪兜底次数（items.xml DualEvTimes 缺失时）。
var studyDualEvHardcode = map[int]int{
	300035: 30,
	300110: 30,
}

func max0(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

// dualEvTimesOf 取道具增加的 LearnTimes；无配置返回 0。
func (s *Server) dualEvTimesOf(itemID int) int {
	if s.cfg.Catalog != nil {
		if n := s.cfg.Catalog.DualEvTimesOf(itemID); n > 0 {
			return n
		}
	}
	return studyDualEvHardcode[itemID]
}

// buildBoostTimesBody 加速器/学习力仪剩余次数 20B：two+three+autoFt+energy+learn。
func buildBoostTimesBody(t store.BoostTimes) []byte {
	out := make([]byte, 20)
	u32 := func(v int) uint32 {
		if v < 0 {
			return 0
		}
		return uint32(v)
	}
	binary.BigEndian.PutUint32(out[0:4], u32(t.TwoTimes))
	binary.BigEndian.PutUint32(out[4:8], u32(t.ThreeTimes))
	binary.BigEndian.PutUint32(out[8:12], u32(t.AutoFightTimes))
	binary.BigEndian.PutUint32(out[12:16], u32(t.EnergyTimes))
	binary.BigEndian.PutUint32(out[16:20], u32(t.LearnTimes))
	return out
}

func (s *Server) boostTimesOf(uid int64) store.BoostTimes {
	if s.cfg.Store == nil {
		return store.BoostTimes{}
	}
	t, err := s.cfg.Store.GetBoostTimes(uid)
	if err != nil {
		return store.BoostTimes{}
	}
	return t
}

// handleUseStudyItem CMD 2332：使用学习力双倍仪。
// 请求 itemID(4)；应答 learnTimes(4)。
func (s *Server) handleUseStudyItem(c *Client, uid uint32, body []byte) {
	itemID := 0
	if len(body) >= 4 {
		itemID = int(binary.BigEndian.Uint32(body[0:4]))
	}
	learn := 0
	if s.cfg.Store != nil {
		t := s.boostTimesOf(int64(uid))
		learn = t.LearnTimes
		addN := s.dualEvTimesOf(itemID)
		if itemID > 0 && addN > 0 {
			n, err := s.cfg.Store.GetItemCount(int64(uid), itemID)
			if err == nil && n > 0 {
				if err := s.cfg.Store.ConsumeItem(int64(uid), itemID, 1); err == nil {
					if left, e := s.cfg.Store.AddLearnTimes(int64(uid), addN); e == nil {
						learn = left
					}
				}
			}
		}
	}
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, uint32(learn))
	s.send(c, 2332, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d item=%d learnTimes=%d", cmdname.Format(2332), uid, itemID, learn)
}

// yieldEVHasAny 六维学习力产出是否非全 0。
func yieldEVHasAny(y [6]int) bool {
	for _, v := range y {
		if v != 0 {
			return true
		}
	}
	return false
}

// scaleYieldEV 按倍率放大产出（双倍仪用）。
func scaleYieldEV(y [6]int, mult int) [6]int {
	if mult <= 1 {
		return y
	}
	for i := 0; i < 6; i++ {
		y[i] *= mult
	}
	return y
}

// shouldGrantYieldingEV 野生、学院训练室、或非经典 SPT Boss 才给学习力。
func shouldGrantYieldingEV(st *BattleState) bool {
	if st == nil || st.EnemyID <= 0 {
		return false
	}
	if isTrainRoomMap(st.MapID) {
		return true
	}
	if st.EnemyCatchable || st.IsWildMonster {
		return true
	}
	_, isSPT := sptPetToAchieveThreshold[st.EnemyID]
	return !isSPT
}

// applyBattleYieldingEV 战后给出战精灵加 YieldingEV；双倍仪触发时扣 LearnTimes。
func (s *Server) applyBattleYieldingEV(uid int64, p *store.Pet, st *BattleState) {
	if s.cfg.Store == nil || p == nil || st == nil || !shouldGrantYieldingEV(st) {
		return
	}
	var yield [6]int
	if y, ok := trainRoomEVYield(st.MapID); ok {
		yield = y
		logTrainEV(uid, st.MapID, yield)
	} else {
		if s.cfg.Catalog == nil {
			return
		}
		yield = s.cfg.Catalog.YieldingEVOf(st.EnemyID)
		if !yieldEVHasAny(yield) {
			return
		}
	}
	if evTotal(p.EV) >= 510 {
		return
	}
	if t := s.boostTimesOf(uid); t.LearnTimes > 0 {
		if ok, left, err := s.cfg.Store.ConsumeLearnTimes(uid, 1); err == nil && ok {
			yield = scaleYieldEV(yield, 2)
			log.Printf("[battle] EV x2 UID=%d learnLeft=%d enemy=%d", uid, left, st.EnemyID)
		}
	}
	before := p.EV
	p.EV = addEVWithCap(p.EV, yield)
	if p.EV == before {
		return
	}
	if err := s.cfg.Store.SetPetEV(uid, p.CatchTime, p.EV); err != nil {
		log.Printf("[battle] WARN SetPetEV UID=%d catch=%d: %v", uid, p.CatchTime, err)
		return
	}
	log.Printf("[battle] YieldingEV UID=%d pet=%d enemy=%d +%v -> %v",
		uid, p.PetID, st.EnemyID, yield, p.EV)
}
