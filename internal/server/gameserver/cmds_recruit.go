package gameserver

import (
	"encoding/binary"
	"log"

	"niaohao/server/internal/cmdname"
	"niaohao/server/internal/store"
)

// 新兵招募计划（RecruitSoilder）——对齐面板文案：
// 满级2只→新兵导弹架；满级4只→新兵奖励(积累经验)；满级6只→招募官套装；满级8只→机械系精灵尤达。
const (
	recruitMaxPetLevel = 100
	recruitPetYoda     = 459
	recruitExpReward   = 20000 // 「新兵奖励」对齐经典档位「两万经验」→经验池
)

var recruitNeedMaxPets = [4]int{2, 4, 6, 8}

var recruitOfficerSuit = []int{100500, 100501, 100502, 100503} // SuitXML 136 招募官

func recruitStatesFromMask(mask uint32) []byte {
	out := make([]byte, 16)
	for i := uint32(0); i < 4; i++ {
		st := uint32(0)
		if mask&(1<<i) != 0 {
			st = 1 // 已领
		}
		binary.BigEndian.PutUint32(out[i*4:(i+1)*4], st)
	}
	return out
}

func (s *Server) countMaxLevelPets(uid int64) int {
	if s.cfg.Store == nil {
		return 0
	}
	n := 0
	for _, listFn := range []func(int64) ([]store.Pet, error){
		s.cfg.Store.ListBagPets,
		s.cfg.Store.ListStoragePets,
	} {
		pets, err := listFn(uid)
		if err != nil {
			continue
		}
		for _, p := range pets {
			if p.Level >= recruitMaxPetLevel {
				n++
			}
		}
	}
	return n
}

// handleGetRecruitStates CMD 70006：4×u32，0=未领 1=已领。
func (s *Server) handleGetRecruitStates(c *Client, uid uint32) {
	mask := uint32(0)
	if s.cfg.Store != nil {
		if m, err := s.cfg.Store.GetRecruitClaimMask(int64(uid)); err == nil {
			mask = m
		}
	}
	out := recruitStatesFromMask(mask)
	s.send(c, 70006, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d mask=%#x maxPets=%d",
		cmdname.Format(70006), uid, mask, s.countMaxLevelPets(int64(uid)))
}

// handleGetRecruitReward CMD 70007：请求 slot(4)；校验满级数后发奖。
func (s *Server) handleGetRecruitReward(c *Client, uid uint32, body []byte) {
	slot := uint32(0)
	if len(body) >= 4 {
		slot = binary.BigEndian.Uint32(body[0:4])
	}
	if slot == 0 {
		slot = 1
	}
	ack := make([]byte, 4)
	binary.BigEndian.PutUint32(ack, slot)

	if slot < 1 || slot > 4 {
		s.send(c, 70007, uid, 0, ack)
		s.sendAlert(int64(uid), "无效的奖励栏位")
		return
	}
	if s.cfg.Store == nil {
		s.send(c, 70007, uid, 0, ack)
		return
	}

	need := recruitNeedMaxPets[slot-1]
	have := s.countMaxLevelPets(int64(uid))
	if have < need {
		s.send(c, 70007, uid, 0, ack)
		s.sendAlert(int64(uid), "满级精灵数量不足，无法领取")
		log.Printf("[CMD] OK     %s UID=%d slot=%d need=%d have=%d", cmdname.Format(70007), uid, slot, need, have)
		return
	}

	mask, err := s.cfg.Store.GetRecruitClaimMask(int64(uid))
	if err != nil {
		s.send(c, 70007, uid, 0, ack)
		log.Printf("[CMD] WARN  %s UID=%d mask: %v", cmdname.Format(70007), uid, err)
		return
	}
	bit := uint32(1) << (slot - 1)
	s.send(c, 70007, uid, 0, ack)
	if mask&bit != 0 {
		s.sendAlert(int64(uid), "该奖励已经领取过了")
		log.Printf("[CMD] OK     %s UID=%d slot=%d already", cmdname.Format(70007), uid, slot)
		return
	}

	switch slot {
	case 1: // 新兵导弹架
		if err := s.cfg.Store.AddItem(int64(uid), 100499, 1); err != nil {
			log.Printf("[CMD] WARN  %s UID=%d AddItem: %v", cmdname.Format(70007), uid, err)
			s.sendAlert(int64(uid), "发放奖励失败")
			return
		}
		s.send(c, 8004, uid, 0, buildBossMonster8004Body(0, 0, 0, 100499, 1))
	case 2: // 新兵奖励 → 积累经验 20000（面板「新兵奖励」；经典档位对齐）
		if _, err := s.cfg.Store.AddExpPool(int64(uid), recruitExpReward); err != nil {
			log.Printf("[CMD] WARN  %s UID=%d AddExpPool: %v", cmdname.Format(70007), uid, err)
			s.sendAlert(int64(uid), "发放奖励失败")
			return
		}
		s.sendAlert(int64(uid), "获得积累经验 20000")
	case 3: // 新兵招募官套装
		for _, id := range recruitOfficerSuit {
			if err := s.cfg.Store.AddItem(int64(uid), id, 1); err != nil {
				log.Printf("[CMD] WARN  %s UID=%d AddItem %d: %v", cmdname.Format(70007), uid, id, err)
				s.sendAlert(int64(uid), "发放奖励失败")
				return
			}
			s.send(c, 8004, uid, 0, buildBossMonster8004Body(0, 0, 0, uint32(id), 1))
		}
	case 4: // 机械系精灵 尤达
		name := "尤达"
		skills := []int{10019}
		if s.cfg.Catalog != nil {
			if n := s.cfg.Catalog.PetNameOf(recruitPetYoda); n != "" {
				name = n
			}
			if moves := s.cfg.Catalog.MovesUpToLevel(recruitPetYoda, 1); len(moves) > 0 {
				skills = skills[:0]
				for _, m := range moves {
					skills = append(skills, m.ID)
				}
			}
		}
		ct, err := s.cfg.Store.GrantPet(int64(uid), recruitPetYoda, name, 1, 20, 0, skills)
		if err != nil {
			log.Printf("[CMD] WARN  %s UID=%d GrantPet: %v", cmdname.Format(70007), uid, err)
			s.sendAlert(int64(uid), "发放精灵失败")
			return
		}
		s.send(c, 8004, uid, 0, buildBossMonster8004Body(0, recruitPetYoda, uint32(ct), 0, 0))
	default:
		return
	}

	mask |= bit
	if err := s.cfg.Store.SetRecruitClaimMask(int64(uid), mask); err != nil {
		log.Printf("[CMD] WARN  %s UID=%d SetMask: %v", cmdname.Format(70007), uid, err)
	}
	log.Printf("[CMD] OK     %s UID=%d slot=%d mask=%#x haveMax=%d", cmdname.Format(70007), uid, slot, mask, have)
}
