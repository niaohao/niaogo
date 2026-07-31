package gameserver

import (
	"sync"

	"niaohao/server/internal/store"
)

var (
	fusionDegradeMapOnce sync.Once
	// key=融合精灵种族线初始ID，value=主融合精灵种族线初始ID
	fusionDegradeMainBaseIDByFusionRootID map[int]int
)

func (s *Server) firstStagePetID(petID int) int {
	if petID <= 0 {
		return 0
	}
	cur := petID
	for i := 0; i < 32; i++ {
		def := s.petBase(cur)
		if def == nil || def.EvolvesFrom <= 0 {
			return cur
		}
		cur = def.EvolvesFrom
	}
	return cur
}

func (s *Server) isFusionPetID(petID int) bool {
	def := s.petBase(petID)
	return def != nil && def.IsFuseMon
}

func (s *Server) buildFusionDegradeMapOnce() {
	fusionDegradeMapOnce.Do(func() {
		fusionDegradeMainBaseIDByFusionRootID = make(map[int]int)
		fusionFormulasMu.RLock()
		formulas := append([]fusionFormula(nil), fusionFormulas...)
		fusionFormulasMu.RUnlock()
		for _, f := range formulas {
			fusionRoot := s.firstStagePetID(f.ResultID)
			mainRoot := s.firstStagePetID(f.MainID)
			if fusionRoot > 0 && mainRoot > 0 {
				if _, exists := fusionDegradeMainBaseIDByFusionRootID[fusionRoot]; !exists {
					fusionDegradeMainBaseIDByFusionRootID[fusionRoot] = mainRoot
				}
			}
		}
		for _, p := range wildBeigePets {
			root := s.firstStagePetID(p)
			if root > 0 {
				if _, exists := fusionDegradeMainBaseIDByFusionRootID[root]; !exists {
					fusionDegradeMainBaseIDByFusionRootID[root] = root
				}
			}
		}
	})
}

func (s *Server) resolveFusionDegradeTargetID(currentPetID int) (int, bool) {
	if !s.isFusionPetID(currentPetID) {
		return 0, false
	}
	fusionRoot := s.firstStagePetID(currentPetID)
	if fusionRoot <= 0 {
		return 0, false
	}
	s.buildFusionDegradeMapOnce()
	if target, ok := fusionDegradeMainBaseIDByFusionRootID[fusionRoot]; ok && target > 0 {
		return target, true
	}
	return fusionRoot, true
}

// applyFusionPetDegrade 融合精灵还原药剂：退回主融合初生并重置初生态。
func (s *Server) applyFusionPetDegrade(p *store.Pet) bool {
	if p == nil {
		return false
	}
	targetID, ok := s.resolveFusionDegradeTargetID(p.PetID)
	if !ok || targetID <= 0 {
		return false
	}
	p.PetID = targetID
	p.Level = 1
	p.Exp = 0
	p.EV = [6]int{}
	p.CurrentHP = 0
	p.Skills = nil
	p.Trait = 0
	p.EnergyBallItemID = 0
	p.EnergyBallLeftCount = 0
	p.EnergyBallEffectID = 0
	p.FormLocked = 0
	p.DisplayFormID = 0
	p.LockedDisplayFormID = 0
	if s.cfg.Catalog != nil {
		if d := s.cfg.Catalog.PetBase(targetID); d != nil && d.Name != "" {
			p.Name = d.Name
		}
	}
	return true
}
