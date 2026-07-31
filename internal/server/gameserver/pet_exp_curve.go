package gameserver

import (
	"math"

	"niaohao/server/internal/store"
)

// expSumSq1To99 = 1²+…+99²；1→100 共 99 次升级。
const expSumSq1To99 = 328350

// growthExpCoeff 成长类型系数（对齐经典：总经验约 125万~150万）。
func growthExpCoeff(growthType int) float64 {
	switch growthType {
	case 0:
		return 1_250_000 / float64(expSumSq1To99)
	case 1:
		return 1_300_000 / float64(expSumSq1To99)
	case 2:
		return 1_350_000 / float64(expSumSq1To99)
	case 3:
		return 1_500_000 / float64(expSumSq1To99)
	default:
		return 1_300_000 / float64(expSumSq1To99)
	}
}

func petGrowthType(petID int) int {
	if d := petBaseFromCatalog(petID); d != nil {
		return d.GrowthType
	}
	return 1
}

// petNextLevelExp 升到下一级所需经验：coeff × level²。
func petNextLevelExp(petID, level int) int {
	if level <= 0 {
		level = 1
	}
	if level >= 100 {
		return 0
	}
	n := int(math.Round(growthExpCoeff(petGrowthType(petID)) * float64(level*level)))
	if n < 1 {
		n = 1
	}
	return n
}

// applyPetExpGain 给精灵加经验并按成长曲线升级；返回实际消耗的经验。
func applyPetExpGain(p *store.Pet, amount int) int {
	if p == nil || amount <= 0 {
		return 0
	}
	if p.Level <= 0 {
		p.Level = 1
	}
	if p.Level >= 100 {
		p.Level = 100
		p.Exp = 0
		return 0
	}
	used := 0
	remain := amount
	for remain > 0 && p.Level < 100 {
		need := petNextLevelExp(p.PetID, p.Level) - p.Exp
		if need <= 0 {
			p.Exp = 0
			p.Level++
			continue
		}
		add := remain
		if add > need {
			add = need
		}
		p.Exp += add
		used += add
		remain -= add
		needFull := petNextLevelExp(p.PetID, p.Level)
		if p.Exp >= needFull {
			p.Exp -= needFull
			p.Level++
		}
	}
	if p.Level >= 100 {
		p.Level = 100
		p.Exp = 0
	}
	return used
}
