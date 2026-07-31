package gameserver

import (
	"encoding/binary"
	"log"

	"niaohao/server/internal/cmdname"
	"niaohao/server/internal/store"
)

// handlePetGetExp CMD 2319：经验分配器当前积累经验。应答 expPool(4)。
func (s *Server) handlePetGetExp(c *Client, uid uint32) {
	pool := 0
	if s.cfg.Store != nil {
		pool, _ = s.cfg.Store.GetExpPool(int64(uid))
	}
	if pool < 0 {
		pool = 0
	}
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, uint32(pool))
	s.send(c, 2319, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d pool=%d", cmdname.Format(2319), uid, pool)
}

// handlePetSetExp CMD 2318：从经验池分配给精灵。
// 请求 catchTime(4)+expAmount(4)；应答 newPool(4)；再推 2508/2507/2301。
func (s *Server) handlePetSetExp(c *Client, uid uint32, body []byte) {
	catchTime, want := uint32(0), uint32(0)
	if len(body) >= 8 {
		catchTime = binary.BigEndian.Uint32(body[0:4])
		want = binary.BigEndian.Uint32(body[4:8])
	}
	pool := 0
	if s.cfg.Store != nil {
		pool, _ = s.cfg.Store.GetExpPool(int64(uid))
	}
	if pool < 0 {
		pool = 0
	}
	use := int(want)
	if use > pool {
		use = pool
	}
	if use < 0 {
		use = 0
	}
	actual := 0
	if s.cfg.Store != nil && catchTime != 0 && use > 0 {
		p, err := s.cfg.Store.GetPetByCatchTime(int64(uid), int64(catchTime))
		if err == nil && p != nil {
			oldLv := p.Level
			actual = applyPetExpGain(p, use)
			note := s.afterPetLevelChange(p, oldLv)
			_ = s.cfg.Store.UpsertPet(p)
			if len(note) > 0 {
				s.send(c, 2507, uid, 0, note)
			}
			s.pushPetPropAndInfo(c, uid, p)
			pool -= actual
			_ = s.cfg.Store.SetExpPool(int64(uid), pool)
		}
	}
	if pool < 0 {
		pool = 0
	}
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, uint32(pool))
	s.send(c, 2318, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d catch=%d want=%d used=%d pool=%d",
		cmdname.Format(2318), uid, catchTime, want, actual, pool)
}

func (s *Server) pushPetPropAndInfo(c *Client, uid uint32, p *store.Pet) {
	if p == nil {
		return
	}
	_, lv, _, hp, atk, def, sa, sd, spd := petCombatStats(p)
	exp := p.Exp
	if lv <= 0 {
		lv = 1
	}
	next := petNextLevelExp(p.PetID, lv)
	prop := buildNoteUpdateProp(uint32(p.CatchTime), p.PetID, lv, exp, exp, next,
		hp, atk, def, sa, sd, spd, p.EV)
	s.send(c, 2508, uid, 0, prop)
	if info := buildPetInfo(p); len(info) > 0 {
		s.send(c, 2301, uid, 0, info)
	}
}

// grantMapPetExp 地图场景给首发/指定精灵加经验；返回实际获得量。
func (s *Server) grantMapPetExp(c *Client, uid uint32, amount int) int {
	if s.cfg.Store == nil || amount <= 0 {
		return 0
	}
	bag, err := s.cfg.Store.ListBagPets(int64(uid))
	if err != nil || len(bag) == 0 {
		return 0
	}
	p := &bag[0]
	oldLv := p.Level
	used := applyPetExpGain(p, amount)
	note := s.afterPetLevelChange(p, oldLv)
	_ = s.cfg.Store.UpsertPet(p)
	if len(note) > 0 {
		s.send(c, 2507, uid, 0, note)
	}
	s.pushPetPropAndInfo(c, uid, p)
	return used
}
