package gameserver

import (
	"encoding/binary"
	"log"

	"niaohao/server/internal/cmdname"
	"niaohao/server/internal/store"
)

// effectiveDisplayID 当前展示形态；无 DisplayFormID 时用能力档 PetID。
func effectiveDisplayID(p *store.Pet) int {
	if p == nil {
		return 0
	}
	if p.DisplayFormID > 0 {
		return p.DisplayFormID
	}
	return p.PetID
}

// petSkinID 写入 PetInfo/PetList 的 skinID（展示形态）。
func petSkinID(p *store.Pet) uint32 {
	if p == nil {
		return 0
	}
	id := effectiveDisplayID(p)
	if id == p.PetID {
		return 0
	}
	return uint32(id)
}

// applyLockedDisplayForm 形态固定后能力档进化仍保持锁定模型。
func applyLockedDisplayForm(p *store.Pet) {
	if p == nil || p.FormLocked == 0 || p.LockedDisplayFormID <= 0 {
		return
	}
	p.DisplayFormID = p.LockedDisplayFormID
}

// handleSetPetConstForm CMD 9113：形态固定胶囊 300152 / 形态还原胶囊 300153。
// 请求 catchTime(4)+arg2(4)；arg2=0 还原展示一档；arg2=当前精灵ID 锁定。
// 应答 UsePetItemOutOfFightInfo，并推 2508+2301。
func (s *Server) handleSetPetConstForm(c *Client, uid uint32, body []byte) {
	fail := func() {
		s.send(c, 9113, uid, 0, nil)
	}
	if s.cfg.Store == nil || len(body) < 8 {
		fail()
		return
	}
	catch := int64(binary.BigEndian.Uint32(body[0:4]))
	arg2 := binary.BigEndian.Uint32(body[4:8])
	p, err := s.cfg.Store.GetPetByCatchTime(int64(uid), catch)
	if err != nil || p == nil {
		fail()
		return
	}
	if s.petBase(p.PetID) == nil {
		fail()
		return
	}

	if arg2 == 0 {
		if err := s.cfg.Store.ConsumeItem(int64(uid), 300153, 1); err != nil {
			fail()
			return
		}
		eff := effectiveDisplayID(p)
		def := s.petBase(eff)
		if def == nil {
			fail()
			return
		}
		nextDisp := 0
		if def.EvolvesFrom > 0 && s.petBase(def.EvolvesFrom) != nil {
			nextDisp = def.EvolvesFrom
		} else {
			cur := eff
			visited := map[int]bool{}
			for {
				if cur <= 0 || visited[cur] {
					break
				}
				visited[cur] = true
				d := s.petBase(cur)
				if d == nil || d.EvolvesTo <= 0 {
					break
				}
				cur = d.EvolvesTo
			}
			if s.petBase(cur) != nil {
				nextDisp = cur
			}
		}
		if nextDisp <= 0 {
			fail()
			return
		}
		p.DisplayFormID = nextDisp
		if p.FormLocked != 0 {
			p.LockedDisplayFormID = nextDisp
		}
		_ = s.cfg.Store.SetPetFormDisplay(int64(uid), catch, p.FormLocked, p.DisplayFormID, p.LockedDisplayFormID)
		s.sendPetForm9113OK(c, uid, p)
		log.Printf("[CMD] OK     %s UID=%d restore display %d->%d catch=%d",
			cmdname.Format(9113), uid, eff, nextDisp, catch)
		return
	}

	if arg2 == uint32(p.PetID) {
		if p.FormLocked != 0 {
			fail()
			return
		}
		if err := s.cfg.Store.ConsumeItem(int64(uid), 300152, 1); err != nil {
			fail()
			return
		}
		lockEff := effectiveDisplayID(p)
		p.FormLocked = 1
		p.LockedDisplayFormID = lockEff
		if lockEff == p.PetID {
			p.DisplayFormID = 0
		} else {
			p.DisplayFormID = lockEff
		}
		_ = s.cfg.Store.SetPetFormDisplay(int64(uid), catch, p.FormLocked, p.DisplayFormID, p.LockedDisplayFormID)
		s.sendPetForm9113OK(c, uid, p)
		log.Printf("[CMD] OK     %s UID=%d lock display=%d catch=%d",
			cmdname.Format(9113), uid, lockEff, catch)
		return
	}
	fail()
}

func (s *Server) sendPetForm9113OK(c *Client, uid uint32, p *store.Pet) {
	if p == nil {
		s.send(c, 9113, uid, 0, nil)
		return
	}
	s.send(c, 9113, uid, 0, buildUsePetItemOutOfFightInfo(p))
	hp, atk, defv, sa, sd, spd := petSixStatsFromPet(p)
	prop := buildNoteUpdateProp(uint32(p.CatchTime), p.PetID, p.Level, p.Exp, p.Exp,
		petNextLevelExp(p.PetID, p.Level), hp, atk, defv, sa, sd, spd, p.EV)
	if len(prop) > 0 {
		s.send(c, 2508, uid, 0, prop)
	}
	if info := buildPetInfo(p); len(info) > 0 {
		s.send(c, 2301, uid, 0, info)
	}
}

