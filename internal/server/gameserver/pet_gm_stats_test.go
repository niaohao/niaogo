package gameserver

import (
	"testing"

	"niaohao/server/internal/store"
)

func TestPetPanelGMOverride(t *testing.T) {
	p := &store.Pet{
		PetID: 1, Level: 50, DV: 31, Nature: 0,
		EV: [6]int{0, 0, 0, 0, 0, 0},
	}
	cur, nat, locked := PetPanelSnapshot(p)
	if locked {
		t.Fatal("should not be locked")
	}
	if cur != nat {
		t.Fatalf("without override current should equal natural: %v vs %v", cur, nat)
	}
	p.HasGMStats = true
	p.GMStats = [6]int{9999, 8888, 7777, 6666, 5555, 4444}
	cur2, nat2, locked2 := PetPanelSnapshot(p)
	if !locked2 {
		t.Fatal("expected locked")
	}
	if cur2 != p.GMStats {
		t.Fatalf("current=%v want %v", cur2, p.GMStats)
	}
	if nat2 != nat {
		t.Fatalf("natural changed: %v vs %v", nat2, nat)
	}
}
