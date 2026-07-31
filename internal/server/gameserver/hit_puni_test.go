package gameserver

import "testing"

func TestCalcHitChance(t *testing.T) {
	if got := calcHitChance(100, 0, 0); got != 100 {
		t.Fatalf("100 base got %d", got)
	}
	if got := calcHitChance(70, 0, 0); got != 70 {
		t.Fatalf("70 base got %d", got)
	}
	if got := calcHitChance(30, 1, 0); got != 55 {
		t.Fatalf("+1 acc stage: got %d want 55", got)
	}
	if got := calcHitChance(100, 0, 1); got != 75 {
		t.Fatalf("+1 eva: got %d want 75", got)
	}
	if got := calcHitChance(100, 6, 0); got != 100 {
		t.Fatalf("cap 100 got %d", got)
	}
	if got := calcHitChance(10, -6, 0); got != 1 {
		t.Fatalf("floor 1 got %d", got)
	}
	if got := calcHitChance(0, 0, 0); got != 100 {
		t.Fatalf("0 accuracy defaults 100 got %d", got)
	}
}

func TestPuniFragmentIDs(t *testing.T) {
	if getPuniFragmentItemID(1) != 400651 {
		t.Fatal(getPuniFragmentItemID(1))
	}
	if getPuniFragmentItemID(8) != 400658 {
		t.Fatal(getPuniFragmentItemID(8))
	}
	if getPuniFragmentItemID(0) != 0 {
		t.Fatal("region 0 should have no fragment")
	}
	if !isPuniSealBoss(514, 300, 3) {
		t.Fatal("514/300/3")
	}
	if !isPuniSealBoss(108, 300, 5) {
		t.Fatal("alias 108")
	}
	if isPuniSealBoss(10, 300, 1) {
		t.Fatal("wrong map")
	}
	if puniSealMaxHP(1) != 5000 {
		t.Fatal(puniSealMaxHP(1))
	}
}
