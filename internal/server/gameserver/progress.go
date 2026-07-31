package gameserver

// applyTowerWinProgress 勇者/试炼塔胜利后层数 +1 落库。
func (s *Server) applyTowerWinProgress(uid int64, kind int) {
	if s.cfg.Store == nil || kind == fightKindNormal {
		return
	}
	switch kind {
	case fightKindBrave:
		cur, _ := s.modes.getBrave(uid)
		if cur < 1 {
			cur = 1
		}
		next := cur + 1
		if next > braveTowerMaxLevel {
			next = braveTowerMaxLevel
		}
		s.modes.setBrave(uid, next, braveTowerBosses(next))
		_ = s.cfg.Store.SetBraveProgress(uid, next)
	case fightKindFresh:
		cur := s.modes.getFresh(uid)
		if cur < 1 {
			cur = 1
		}
		next := cur + 1
		if next > freshTowerMaxLevel {
			next = freshTowerMaxLevel
		}
		s.modes.setFresh(uid, next)
		_ = s.cfg.Store.SetFreshProgress(uid, next)
	}
}

// loadUserProgress 登录/进塔前恢复会话层数。
func (s *Server) loadUserProgress(uid int64) {
	if s.cfg.Store == nil {
		return
	}
	p, err := s.cfg.Store.GetProgress(uid)
	if err != nil {
		return
	}
	s.modes.setBrave(uid, p.BraveCur, braveTowerBosses(p.BraveCur))
	s.modes.setFresh(uid, p.FreshCur)
}

func (s *Server) loginProgress(uid int64) (braveCur, braveMax, freshCur, freshMax uint32) {
	braveCur, braveMax, freshCur, freshMax = 1, 1, 1, 1
	if s.cfg.Store == nil {
		return
	}
	p, err := s.cfg.Store.GetProgress(uid)
	if err != nil {
		return
	}
	if p.BraveCur > 0 {
		braveCur = uint32(p.BraveCur)
	}
	if p.BraveMax > 0 {
		braveMax = uint32(p.BraveMax)
	}
	if p.FreshCur > 0 {
		freshCur = uint32(p.FreshCur)
	}
	if p.FreshMax > 0 {
		freshMax = uint32(p.FreshMax)
	}
	return
}
