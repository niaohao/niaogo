package gameserver

import (
	"math/rand"

	"niaohao/server/internal/store"
)

// applyEnergyBallBonus 按 NewSeIdx（EnergyBallEffectID）叠加对战属性。
// Eid=26：Args[statKind,value]；1防 2特防 3攻 4特攻 5速度。
// Eid=30：Args[n] 致命率 +n/16（基础按 1/16）。
// 赛尔间对战无效——本服仅 PvE，一律生效。
func (s *Server) applyEnergyBallBonus(p *store.Pet, atk, def, sa, sd, spd int) (oAtk, oDef, oSA, oSD, oSpd, critSixteenths int) {
	oAtk, oDef, oSA, oSD, oSpd = atk, def, sa, sd, spd
	if p == nil || p.EnergyBallLeftCount <= 0 || p.EnergyBallEffectID <= 0 {
		return
	}
	if s.cfg.Catalog == nil {
		return
	}
	d, ok := s.cfg.Catalog.PetEffectByIdx(p.EnergyBallEffectID)
	if !ok {
		return
	}
	switch d.Eid {
	case 26:
		if len(d.Args) < 2 {
			return
		}
		v := d.Args[1]
		switch d.Args[0] {
		case 1:
			oDef += v
		case 2:
			oSD += v
		case 3:
			oAtk += v
		case 4:
			oSA += v
		case 5:
			oSpd += v
		}
	case 30:
		if len(d.Args) >= 1 && d.Args[0] > 0 {
			critSixteenths = d.Args[0]
		}
	}
	return
}

// rollPlayerCrit 基础 1/16，能量珠 Eid=30 再加 n/16。
func rollPlayerCrit(extraSixteenths int) bool {
	n := 1 + extraSixteenths
	if n < 1 {
		n = 1
	}
	if n > 16 {
		n = 16
	}
	return rand.Intn(16) < n
}
