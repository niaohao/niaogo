package gameserver

import (
	"math"
	"path/filepath"
	"testing"

	"niaohao/server/internal/tableloader"
)

func TestCalcCatchProbability(t *testing.T) {
	// 皮皮 CatchRate=255、普通胶囊 Bonus=1、满血 → 约 30%
	p := calcCatchProbability(255, 11, 11, 1, false)
	if math.Abs(p-0.3) > 0.001 {
		t.Fatalf("full hp pipi want~0.3 got %.4f", p)
	}
	p2 := calcCatchProbability(255, 1, 11, 1, false)
	want := (255.0 / 255.0) * (0.3 + 0.7*(10.0/11.0)) * 1
	if want > 0.99 {
		want = 0.99
	}
	if math.Abs(p2-want) > 0.001 {
		t.Fatalf("low hp want %.4f got %.4f", want, p2)
	}
	if calcCatchProbability(1, 100, 100, 256, false) != 1 {
		t.Fatal("master ball should be 1")
	}
	if calcCatchProbability(45, 10, 20, 2, false) <= calcCatchProbability(45, 10, 20, 1, false) {
		t.Fatal("higher bonus should raise prob")
	}
	base := calcCatchProbability(255, 11, 11, 1, false)
	boosted := calcCatchProbability(255, 11, 11, 1, true)
	if boosted <= base {
		t.Fatal("status control should boost catch")
	}
}

func TestEnemyCombatStatsPipiLv1(t *testing.T) {
	xmlDir := filepath.Join("..", "..", "..", "tables", "xml")
	cat := tableloader.New(xmlDir)
	if err := cat.Load(); err != nil {
		t.Skip(err)
	}
	old := defaultSkillCatalog
	defaultSkillCatalog = cat
	defer func() { defaultSkillCatalog = old }()

	hp, atk, _, _, _, _ := enemyCombatStats(10, 1)
	// 真实 Lv1 皮皮约十几血、攻击个位数级，绝非 stub 的 36/37
	if hp < 10 || hp > 20 {
		t.Fatalf("pipi lv1 hp=%d want 10..20", hp)
	}
	if atk > 15 {
		t.Fatalf("pipi lv1 atk=%d too high (stub was ~37)", atk)
	}
}

func TestCatalogCatchRateAndCapsuleBonus(t *testing.T) {
	xmlDir := filepath.Join("..", "..", "..", "tables", "xml")
	cat := tableloader.New(xmlDir)
	if err := cat.Load(); err != nil {
		t.Skip(err)
	}
	if cat.CatchRateOf(10) != 255 {
		t.Fatalf("pipi CatchRate=%d", cat.CatchRateOf(10))
	}
	if cat.ItemCatchBonusOf(300001) != 1 {
		t.Fatalf("normal capsule bonus=%v", cat.ItemCatchBonusOf(300001))
	}
	if cat.ItemCatchBonusOf(300006) != 256 {
		t.Fatalf("master capsule bonus=%v", cat.ItemCatchBonusOf(300006))
	}
}
