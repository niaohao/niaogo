package gameserver

import (
	"path/filepath"
	"runtime"
	"testing"

	"niaohao/server/internal/store"
	"niaohao/server/internal/tableloader"
)

func testXMLDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "tables", "xml")
}

func testCatalog(t *testing.T) *tableloader.Catalog {
	t.Helper()
	c := tableloader.New(testXMLDir(t))
	if err := c.Load(); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestApplyEnergyBallBonusStat(t *testing.T) {
	s := &Server{cfg: Config{Catalog: testCatalog(t)}}
	p := &store.Pet{EnergyBallLeftCount: 5, EnergyBallEffectID: 1003}
	atk, def, sa, sd, spd, crit := s.applyEnergyBallBonus(p, 100, 80, 90, 70, 60)
	if atk != 120 || def != 80 || sa != 90 || sd != 70 || spd != 60 || crit != 0 {
		t.Fatalf("Eid=26 atk: got %d %d %d %d %d crit=%d", atk, def, sa, sd, spd, crit)
	}
}

func TestApplyEnergyBallBonusCrit(t *testing.T) {
	s := &Server{cfg: Config{Catalog: testCatalog(t)}}
	p := &store.Pet{EnergyBallLeftCount: 3, EnergyBallEffectID: 1051}
	_, _, _, _, _, crit := s.applyEnergyBallBonus(p, 50, 50, 50, 50, 50)
	if crit != 1 {
		t.Fatalf("Eid=30 crit bonus=%d want 1", crit)
	}
}

func TestApplyEnergyBallBonusNoEffect(t *testing.T) {
	s := &Server{cfg: Config{Catalog: testCatalog(t)}}
	p := &store.Pet{EnergyBallLeftCount: 0, EnergyBallEffectID: 1001}
	atk, _, _, _, _, crit := s.applyEnergyBallBonus(p, 100, 80, 90, 70, 60)
	if atk != 100 || crit != 0 {
		t.Fatal("no bonus when exhausted")
	}
}

func TestRollPlayerCrit(t *testing.T) {
	if !rollPlayerCrit(15) {
		t.Fatal("crit 16/16")
	}
}
