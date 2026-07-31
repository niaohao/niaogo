package gameserver

import "sync"

// petHPHub 会话缓存出战 HP；持久化走 pets.extra_json.currentHp（0=满血）。
type petHPHub struct {
	mu sync.Mutex
	m  map[int64]map[uint32]int32 // uid -> catchTime -> hp；负数表示满血
}

func (h *petHPHub) tryGet(uid int64, catch uint32) (hp int32, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.m == nil || h.m[uid] == nil {
		return 0, false
	}
	v, ok := h.m[uid][catch]
	return v, ok
}

func (h *petHPHub) get(uid int64, catch uint32, maxHP uint32) uint32 {
	v, ok := h.tryGet(uid, catch)
	if !ok || v < 0 {
		return maxHP
	}
	if uint32(v) > maxHP {
		return maxHP
	}
	return uint32(v)
}

func (h *petHPHub) set(uid int64, catch uint32, hp uint32) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.m == nil {
		h.m = make(map[int64]map[uint32]int32)
	}
	if h.m[uid] == nil {
		h.m[uid] = make(map[uint32]int32)
	}
	h.m[uid][catch] = int32(hp)
}

func (h *petHPHub) clearPet(uid int64, catch uint32) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.m != nil && h.m[uid] != nil {
		delete(h.m[uid], catch)
	}
}

// recalledPetHP：会话缓存 → DB currentHp → 满血。
func (s *Server) recalledPetHP(uid int64, catch uint32, maxHP uint32) uint32 {
	if v, ok := s.petHP.tryGet(uid, catch); ok {
		if v < 0 {
			return maxHP
		}
		if uint32(v) > maxHP {
			return maxHP
		}
		if v == 0 {
			return maxHP
		}
		return uint32(v)
	}
	if s.cfg.Store != nil && catch > 0 {
		if p, err := s.cfg.Store.GetPetByCatchTime(uid, int64(catch)); err == nil && p != nil && p.CurrentHP > 0 {
			hp := uint32(p.CurrentHP)
			if hp > maxHP {
				hp = maxHP
			}
			s.petHP.set(uid, catch, hp)
			return hp
		}
	}
	return maxHP
}

func (s *Server) rememberPetHP(uid int64, catch uint32, hp uint32) {
	if catch == 0 {
		return
	}
	s.petHP.set(uid, catch, hp)
	if s.cfg.Store != nil {
		_ = s.cfg.Store.SetPetCurrentHP(uid, int64(catch), int(hp))
	}
}

func (s *Server) forgetPetHP(uid int64, catch uint32) {
	if catch == 0 {
		return
	}
	s.petHP.clearPet(uid, catch)
	if s.cfg.Store != nil {
		_ = s.cfg.Store.SetPetCurrentHP(uid, int64(catch), 0)
	}
}
