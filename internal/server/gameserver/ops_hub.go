package gameserver

import (
	"time"

	"niaohao/server/internal/store"
)

func (s *Server) loadUserOps(uid int64) store.UserOpsState {
	st := store.UserOpsState{}
	if s.cfg.Store != nil {
		raw, _ := s.cfg.Store.GetUserOps(uid)
		st = raw
	}
	return store.NormalizeUserOps(st, time.Now())
}

func (s *Server) saveUserOps(uid int64, st store.UserOpsState) {
	if s.cfg.Store == nil {
		return
	}
	_ = s.cfg.Store.SetUserOps(uid, store.NormalizeUserOps(st, time.Now()))
}

func (s *Server) tryMarkDaily(uid int64, key string) bool {
	st := s.loadUserOps(uid)
	if st.Daily[key] > 0 {
		return false
	}
	st.Daily[key] = 1
	s.saveUserOps(uid, st)
	return true
}

func (s *Server) dailyCount(uid int64, key string) int {
	return s.loadUserOps(uid).Daily[key]
}

func (s *Server) bumpDaily(uid int64, key string) int {
	st := s.loadUserOps(uid)
	st.Daily[key]++
	n := st.Daily[key]
	s.saveUserOps(uid, st)
	return n
}

func (s *Server) tryMarkWeekly(uid int64, key string) bool {
	st := s.loadUserOps(uid)
	if st.Weekly[key] > 0 {
		return false
	}
	st.Weekly[key] = 1
	s.saveUserOps(uid, st)
	return true
}

func (s *Server) bumpWeekly(uid int64, key string) int {
	st := s.loadUserOps(uid)
	st.Weekly[key]++
	n := st.Weekly[key]
	s.saveUserOps(uid, st)
	return n
}

func (s *Server) weeklyCount(uid int64, key string) int {
	return s.loadUserOps(uid).Weekly[key]
}

func (s *Server) tryMarkMonthly(uid int64, key string) bool {
	st := s.loadUserOps(uid)
	if st.Monthly[key] > 0 {
		return false
	}
	st.Monthly[key] = 1
	s.saveUserOps(uid, st)
	return true
}

func (s *Server) monthlyCount(uid int64, key string) int {
	return s.loadUserOps(uid).Monthly[key]
}

func (s *Server) lifetimeCount(uid int64, key string) int {
	return s.loadUserOps(uid).Lifetime[key]
}

func (s *Server) bumpLifetime(uid int64, key string) int {
	st := s.loadUserOps(uid)
	st.Lifetime[key]++
	n := st.Lifetime[key]
	s.saveUserOps(uid, st)
	return n
}

func (s *Server) tryMarkLifetime(uid int64, key string) bool {
	st := s.loadUserOps(uid)
	if st.Lifetime[key] > 0 {
		return false
	}
	st.Lifetime[key] = 1
	s.saveUserOps(uid, st)
	return true
}

func (s *Server) setLifetime(uid int64, key string, n int) {
	st := s.loadUserOps(uid)
	if n < 0 {
		n = 0
	}
	st.Lifetime[key] = n
	s.saveUserOps(uid, st)
}

func (s *Server) addHonor(uid int64, n int) int {
	st := s.loadUserOps(uid)
	st.Honor += n
	if st.Honor < 0 {
		st.Honor = 0
	}
	s.saveUserOps(uid, st)
	return st.Honor
}

func (s *Server) getHonor(uid int64) int {
	return s.loadUserOps(uid).Honor
}

func (s *Server) lastFusionTrait(uid int64, recipe string) int {
	return s.loadUserOps(uid).FusionTraits[recipe]
}

func (s *Server) setFusionTrait(uid int64, recipe string, trait int) {
	st := s.loadUserOps(uid)
	st.FusionTraits[recipe] = trait
	s.saveUserOps(uid, st)
}

// chinaDayKey 保留给签到等；日常计数用 store.ChinaGameDayKey（6:00 换日）。
func chinaDayKey(now time.Time) string {
	return store.ChinaGameDayKey(now)
}
