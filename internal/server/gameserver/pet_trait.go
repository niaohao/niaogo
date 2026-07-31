package gameserver

import (
	"math/rand"

	"niaohao/server/internal/store"
)

const (
	petTraitMinIdx = 1006
	petTraitMaxIdx = 1045
)

// IsValidPetTrait 融合/普通特性 Idx 是否在可识别范围。
func IsValidPetTrait(trait int) bool {
	return trait >= petTraitMinIdx && trait <= petTraitMaxIdx
}

// RollPetTrait 随机一条特性（1006-1045）。
func RollPetTrait() int {
	return petTraitMinIdx + rand.Intn(petTraitMaxIdx-petTraitMinIdx+1)
}

// RollPetTraitAvoid 融合不重复：避开上次同配方特性（无保底，仍可能回到更早出现的）。
func RollPetTraitAvoid(avoid int) int {
	if !IsValidPetTrait(avoid) {
		return RollPetTrait()
	}
	for i := 0; i < 16; i++ {
		t := RollPetTrait()
		if t != avoid {
			return t
		}
	}
	return RollPetTrait()
}

// AssignPetTraitIfNeeded 若尚未分配则按 catchTime+petID 稳定随机一条特性。
// 仅用于「开启特性」道具等显式开启路径，发宠/捕捉不得调用。
func AssignPetTraitIfNeeded(p *store.Pet) {
	if p == nil || p.PetID <= 0 {
		return
	}
	if IsValidPetTrait(p.Trait) {
		return
	}
	seed := int64(p.CatchTime)<<32 | int64(p.PetID)
	r := rand.New(rand.NewSource(seed))
	p.Trait = petTraitMinIdx + r.Intn(petTraitMaxIdx-petTraitMinIdx+1)
}

// RerollPetTrait 强制重随（特性重组剂）；avoid 时尽量避开上次。
func RerollPetTrait(p *store.Pet, avoidLast bool) {
	if p == nil {
		return
	}
	old := p.Trait
	for i := 0; i < 8; i++ {
		p.Trait = RollPetTrait()
		if !avoidLast || p.Trait != old || petTraitMaxIdx <= petTraitMinIdx {
			return
		}
	}
}

// applyBattlePetTrait 开战/换宠：仅使用已开启的特性（本前端需晶片/融合写入），不懒分配。
func (s *Server) applyBattlePetTrait(_ int64, p *store.Pet, critBonus *int) int {
	if p == nil || !IsValidPetTrait(p.Trait) {
		return 0
	}
	if critBonus != nil {
		*critBonus += traitCritBonusSixteenths(p.Trait)
	}
	return p.Trait
}
