package gameserver

import (
	"niaohao/server/internal/store"
	"niaohao/server/internal/tableloader"
)

// canDirectEvolve 升级后可直接进化：有 EvolvesTo、达等级、无需道具、非进化舱专属。
func canDirectEvolve(cat *tableloader.Catalog, petID, level int) (targetID int, ok bool) {
	if cat == nil || petID <= 0 {
		return 0, false
	}
	d := cat.PetBase(petID)
	if d == nil || d.EvolvesTo <= 0 {
		return 0, false
	}
	if d.EvolvingLv > 0 && level < d.EvolvingLv {
		return 0, false
	}
	if d.EvolvItem > 0 {
		return 0, false
	}
	if d.EvolveBabin == 1 {
		return 0, false
	}
	return d.EvolvesTo, true
}

// tryDirectEvolvePet 按当前等级尝试直接进化（可连锁）；成功则改 PetID、清 Exp、刷新名与锁定形态。
func tryDirectEvolvePet(cat *tableloader.Catalog, p *store.Pet) bool {
	if p == nil || cat == nil {
		return false
	}
	changed := false
	for i := 0; i < 8; i++ {
		to, ok := canDirectEvolve(cat, p.PetID, p.Level)
		if !ok {
			break
		}
		p.PetID = to
		p.Exp = 0
		if n := cat.PetNameOf(to); n != "" {
			p.Name = n
		}
		applyLockedDisplayForm(p)
		changed = true
	}
	return changed
}

func (s *Server) tryDirectEvolve(p *store.Pet) bool {
	cat := (*tableloader.Catalog)(nil)
	if s != nil {
		cat = s.cfg.Catalog
	}
	if cat == nil {
		cat = defaultSkillCatalog
	}
	return tryDirectEvolvePet(cat, p)
}

// afterPetLevelChange 升级后：直接进化 → 学技 → 补满技能槽；返回 2507 包体。
func (s *Server) afterPetLevelChange(p *store.Pet, oldLevel int) []byte {
	if p == nil {
		return nil
	}
	s.tryDirectEvolve(p)
	var note []byte
	if p.Level > oldLevel {
		note = s.applyLevelUpSkills(p, oldLevel)
	}
	s.fillPetSkillsUpToFour(p)
	return note
}
