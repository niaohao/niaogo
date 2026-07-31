package gameserver

import (
	"encoding/binary"
	"log"

	"niaohao/server/internal/cmdname"
)

var speedupDualHardcode = map[int]int{
	300027: 50, 300067: 25, 300101: 10, 300111: 50,
}
var speedupTripleHardcode = map[int]int{
	300051: 50, 300115: 50,
}
var energyAbsorbHardcode = map[int]int{
	300029: 40, 300116: 40,
}
var autoFightBtlHardcode = map[int]int{
	300028: 100, 300068: 50, 300713: 100,
}

func (s *Server) speedupItemEffect(itemID int) (dual, triple int) {
	if s.cfg.Catalog != nil {
		dual = s.cfg.Catalog.DualExpTimesOf(itemID)
		triple = s.cfg.Catalog.TrinalExpTimesOf(itemID)
	}
	if dual == 0 {
		dual = speedupDualHardcode[itemID]
	}
	if triple == 0 {
		triple = speedupTripleHardcode[itemID]
	}
	return
}

func (s *Server) energyAbsorbEffect(itemID int) int {
	if s.cfg.Catalog != nil {
		if n := s.cfg.Catalog.EnergyAbsTimesOf(itemID); n > 0 {
			return n
		}
	}
	return energyAbsorbHardcode[itemID]
}

func (s *Server) autoFightBtlRounds(itemID int) int {
	if s.cfg.Catalog != nil {
		if n := s.cfg.Catalog.AutoBtlTimesOf(itemID); n > 0 {
			return n
		}
	}
	return autoFightBtlHardcode[itemID]
}

// handleUseSpeedupItem CMD 2327：双倍/三倍经验加速器。
// 请求 itemID(4)；应答 twoTimes(4)+threeTimes(4)。
func (s *Server) handleUseSpeedupItem(c *Client, uid uint32, body []byte) {
	itemID := 0
	if len(body) >= 4 {
		itemID = int(binary.BigEndian.Uint32(body[0:4]))
	}
	two, three := 0, 0
	if s.cfg.Store != nil {
		t := s.boostTimesOf(int64(uid))
		two, three = t.TwoTimes, t.ThreeTimes
		addDual, addTriple := s.speedupItemEffect(itemID)
		if itemID > 0 && (addDual > 0 || addTriple > 0) {
			n, err := s.cfg.Store.GetItemCount(int64(uid), itemID)
			if err == nil && n > 0 {
				if err := s.cfg.Store.ConsumeItem(int64(uid), itemID, 1); err == nil {
					if addTriple > 0 {
						if v, e := s.cfg.Store.AddThreeTimes(int64(uid), addTriple); e == nil {
							three = v
						}
					}
					if addDual > 0 {
						if v, e := s.cfg.Store.AddTwoTimes(int64(uid), addDual); e == nil {
							two = v
						}
					}
				}
			}
		}
	}
	out := make([]byte, 8)
	binary.BigEndian.PutUint32(out[0:4], uint32(two))
	binary.BigEndian.PutUint32(out[4:8], uint32(three))
	s.send(c, 2327, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d item=%d two=%d three=%d", cmdname.Format(2327), uid, itemID, two, three)
}

// handleUseAutoFightItem CMD 2329：自动战斗器道具。
// 请求 itemID(4)；应答 autoFight(4)+autoFightTimes(4)。
func (s *Server) handleUseAutoFightItem(c *Client, uid uint32, body []byte) {
	itemID := 0
	if len(body) >= 4 {
		itemID = int(binary.BigEndian.Uint32(body[0:4]))
	}
	af, times := 0, 0
	if s.cfg.Store != nil {
		t := s.boostTimesOf(int64(uid))
		af, times = t.AutoFight, t.AutoFightTimes
		add := s.autoFightBtlRounds(itemID)
		if itemID > 0 && add > 0 {
			n, err := s.cfg.Store.GetItemCount(int64(uid), itemID)
			if err == nil && n > 0 {
				if err := s.cfg.Store.ConsumeItem(int64(uid), itemID, 1); err == nil {
					if v, e := s.cfg.Store.AddAutoFightTimes(int64(uid), add); e == nil {
						times = v
					}
					if af != 3 && times > 0 {
						af = 1
						_ = s.cfg.Store.SetAutoFight(int64(uid), 1)
					}
				}
			}
		}
	}
	out := make([]byte, 8)
	binary.BigEndian.PutUint32(out[0:4], uint32(af))
	binary.BigEndian.PutUint32(out[4:8], uint32(times))
	s.send(c, 2329, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d item=%d af=%d times=%d", cmdname.Format(2329), uid, itemID, af, times)
}

// handleOnOffAutoFight CMD 2330：开关自动战斗。
// 请求 param(4) 1=开→3 / 0=关→有次数则1否则0；应答同 2329。
func (s *Server) handleOnOffAutoFight(c *Client, uid uint32, body []byte) {
	param := uint32(0)
	if len(body) >= 4 {
		param = binary.BigEndian.Uint32(body[0:4])
	}
	af, times := 0, 0
	if s.cfg.Store != nil {
		t := s.boostTimesOf(int64(uid))
		times = t.AutoFightTimes
		if param == 1 {
			if times > 0 {
				af = 3
			} else {
				af = t.AutoFight
			}
		} else {
			if times > 0 {
				af = 1
			} else {
				af = 0
			}
		}
		_ = s.cfg.Store.SetAutoFight(int64(uid), af)
	}
	out := make([]byte, 8)
	binary.BigEndian.PutUint32(out[0:4], uint32(af))
	binary.BigEndian.PutUint32(out[4:8], uint32(times))
	s.send(c, 2330, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d param=%d af=%d times=%d", cmdname.Format(2330), uid, param, af, times)
}

// handleUseEnergyXishou CMD 2331：能量吸收器。
// 请求 itemID(4)；应答 energyTimes(4)。
func (s *Server) handleUseEnergyXishou(c *Client, uid uint32, body []byte) {
	itemID := 0
	if len(body) >= 4 {
		itemID = int(binary.BigEndian.Uint32(body[0:4]))
	}
	energy := 0
	if s.cfg.Store != nil {
		t := s.boostTimesOf(int64(uid))
		energy = t.EnergyTimes
		addN := s.energyAbsorbEffect(itemID)
		if itemID > 0 && addN > 0 {
			n, err := s.cfg.Store.GetItemCount(int64(uid), itemID)
			if err == nil && n > 0 {
				if err := s.cfg.Store.ConsumeItem(int64(uid), itemID, 1); err == nil {
					if v, e := s.cfg.Store.AddEnergyTimes(int64(uid), addN); e == nil {
						energy = v
					}
				}
			}
		}
	}
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, uint32(energy))
	s.send(c, 2331, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d item=%d energy=%d", cmdname.Format(2331), uid, itemID, energy)
}

// battleExpMultiplier 三倍优先于双倍；返回倍率（调用方负责扣次）。
func battleExpMultiplier(two, three int) int {
	if three > 0 {
		return 3
	}
	if two > 0 {
		return 2
	}
	return 1
}

// consumeBattleExpBoost 按倍率扣对应场数。
func (s *Server) consumeBattleExpBoost(uid int64, mult int) {
	if s.cfg.Store == nil || mult <= 1 {
		return
	}
	if mult >= 3 {
		_, _, _ = s.cfg.Store.ConsumeThreeTimes(uid, 1)
		return
	}
	_, _, _ = s.cfg.Store.ConsumeTwoTimes(uid, 1)
}

// shouldConsumeAutoFight 野生或非经典 SPT 的 PvE 扣自动战斗次数。
func shouldConsumeAutoFight(st *BattleState) bool {
	if st == nil || st.isPvP() {
		return false
	}
	return shouldGrantYieldingEV(st)
}

// consumeAutoFightOnEnter 2404 进场：已开启(autoFight=3)则扣 1。
func (s *Server) consumeAutoFightOnEnter(uid int64, st *BattleState) {
	if s.cfg.Store == nil || !shouldConsumeAutoFight(st) {
		return
	}
	t := s.boostTimesOf(uid)
	if t.AutoFight != 3 || t.AutoFightTimes <= 0 {
		return
	}
	ok, left, err := s.cfg.Store.ConsumeAutoFightTimes(uid, 1)
	if err != nil || !ok {
		return
	}
	log.Printf("[battle] autoFight -1 UID=%d left=%d", uid, left)
}
