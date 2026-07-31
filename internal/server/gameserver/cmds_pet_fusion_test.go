package gameserver

import (
	"path/filepath"
	"testing"

	"niaohao/server/internal/store"
	"niaohao/server/internal/tableloader"
)

func TestFusionFormulasExpanded(t *testing.T) {
	xmlDir := filepath.Join("..", "..", "..", "tables", "xml")
	tryLoadFusionFormulas(xmlDir)
	if fusionFormulaCount() < 30 {
		t.Fatalf("expected expanded formulas, got %d", fusionFormulaCount())
	}
	// 副宠叮叮(28) 也应命中绿色珠
	bead, result, ok := matchFusionFormula(3, 28)
	if !ok || bead != 1000001 || result != 301 {
		t.Fatalf("3+28 => want 1000001/301, got ok=%v bead=%d result=%d", ok, bead, result)
	}
	// 灰色珠：雷吉欧斯+奇塔
	bead, result, ok = matchFusionFormula(61, 102)
	if !ok || bead != 1000010 || result != 338 {
		t.Fatalf("61+102 => want 1000010/338, got ok=%v bead=%d result=%d", ok, bead, result)
	}
	// 橙紫：萨洛姆斯+伊娃
	bead, result, ok = matchFusionFormula(251, 232)
	if !ok || bead != 1000014 || result != 427 {
		t.Fatalf("251+232 => want 1000014/427, got ok=%v bead=%d result=%d", ok, bead, result)
	}
}

func TestHatchAbsorbStepsPink(t *testing.T) {
	xmlDir := filepath.Join("..", "..", "..", "tables", "xml")
	cat := tableloader.New(xmlDir)
	if err := tableloader.LoadHatchTaskXML(cat, xmlDir); err != nil {
		t.Fatal(err)
	}
	if n := cat.HatchAbsorbSteps(1000009); n != 3 {
		t.Fatalf("pink bead steps want 3 got %d", n)
	}
	if n := cat.HatchAbsorbSteps(1000001); n != 1 {
		t.Fatalf("green bead steps want 1 got %d", n)
	}
	s := &Server{cfg: Config{Catalog: cat}}
	b := &store.SoulBead{ItemID: 1000009}
	b.Status[0] = true
	if s.soulBeadAbsorbComplete(b) {
		t.Fatal("pink bead incomplete with 1/3 steps")
	}
	b.Status[1], b.Status[2] = true, true
	if !s.soulBeadAbsorbComplete(b) {
		t.Fatal("pink bead should complete with 3/3")
	}
}

func TestSoulBeadDefFromItems(t *testing.T) {
	xmlDir := filepath.Join("..", "..", "..", "tables", "xml")
	cat := tableloader.New(xmlDir)
	if err := cat.Load(); err != nil {
		t.Fatal(err)
	}
	d, ok := cat.SoulBeadOf(1000007)
	if !ok || len(d.TransmuteMon) < 3 {
		t.Fatalf("beige bead TransmuteMon incomplete: %+v", d)
	}
	d2, ok := cat.SoulBeadOf(1000010)
	if !ok || d2.TransmuteMon[0] != 338 {
		t.Fatalf("gray bead want mon 338 got %+v", d2)
	}
}

func TestChooseFusionNature(t *testing.T) {
	if chooseFusionNature(5, 5) != 5 {
		t.Fatal("same nature should inherit")
	}
	n := chooseFusionNature(3, 7)
	if n < 0 || n > 24 {
		t.Fatalf("random nature out of range: %d", n)
	}
}
