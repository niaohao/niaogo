package gameserver

import (
	"log"
	"math"
)

// 回收价=系数×(DV+1)×(1+等级/200)。缺《回收价格表》时按 PetClass 给默认系数。
const (
	recycleDefaultCoef = 80
	recycleRareCoef    = 220
	recycleHonorDaily  = 5 // 每日首次回收额外荣誉（攻略）
)

func (s *Server) petRecycleCoef(petID int) int {
	if s.cfg.Catalog == nil {
		return recycleDefaultCoef
	}
	d := s.cfg.Catalog.PetBase(petID)
	if d == nil {
		return recycleDefaultCoef
	}
	if d.IsRareMon {
		return recycleRareCoef
	}
	switch {
	case d.PetClass >= 100:
		return 150
	case d.PetClass >= 50:
		return 120
	default:
		return recycleDefaultCoef
	}
}

func calcPetRecycleCoins(coef, dv, level int) int {
	if coef <= 0 {
		coef = recycleDefaultCoef
	}
	if dv < 0 {
		dv = 0
	}
	if level < 1 {
		level = 1
	}
	v := float64(coef) * float64(dv+1) * (1 + float64(level)/200)
	n := int(math.Floor(v + 0.5))
	if n < 1 {
		n = 1
	}
	return n
}

func (s *Server) isPetRecycleForbidden(petID int) bool {
	if s.cfg.Catalog == nil {
		return false
	}
	d := s.cfg.Catalog.PetBase(petID)
	return d != nil && d.FreeForbidden
}

// grantPetRecycleReward 放生仓转移时发豆；FreeForbidden / 超No过期不发豆；每日首次另+5荣誉。
func (s *Server) grantPetRecycleReward(uid int64, petID, dv, level int) (coins int, honor int) {
	if s.cfg.Store == nil || petID <= 0 || s.isPetRecycleForbidden(petID) {
		return 0, 0
	}
	// 攻略：超能 NoNo 到期后放生不再获得赛尔豆
	if !s.hasActiveSuperNono(uid) {
		if s.tryMarkDaily(uid, "recycleHonor") {
			honor = recycleHonorDaily
			s.addHonor(uid, honor)
		}
		return 0, honor
	}
	coins = calcPetRecycleCoins(s.petRecycleCoef(petID), dv, level)
	if coins > 0 {
		if err := s.cfg.Store.AddCoins(uid, coins); err != nil {
			log.Printf("[recycle] AddCoins uid=%d +%d: %v", uid, coins, err)
			coins = 0
		}
	}
	if s.tryMarkDaily(uid, "recycleHonor") {
		honor = recycleHonorDaily
		s.addHonor(uid, honor)
	}
	return coins, honor
}
