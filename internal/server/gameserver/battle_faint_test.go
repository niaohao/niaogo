package gameserver

import (
	"testing"

	"niaohao/server/internal/store"
)

func TestFaintedPetBlocksSwitchAndEnds(t *testing.T) {
	st := &BattleState{PlayerCatchTime: 1001, PlayerHP: 0, Active: true}
	st.markPetFainted(1001)
	st.markPetFainted(1002)
	if !st.isPetFainted(1001) || !st.isPetFainted(1002) {
		t.Fatal("mark")
	}
	if st.allowsPetSwitch() != true {
		t.Fatal("pve allows switch")
	}
	st.ForceSinglePet = true
	if st.allowsPetSwitch() {
		t.Fatal("force single")
	}
	st.ForceSinglePet = false
	st.OpponentUID = 2
	st.PvPMode = pvpModeSingle
	if st.allowsPetSwitch() {
		t.Fatal("pvp single")
	}

	bag := []store.Pet{
		{CatchTime: 1001},
		{CatchTime: 1002},
		{CatchTime: 1003},
	}
	s := &Server{}
	st.OpponentUID = 0
	st.PvPMode = 0
	st.FaintedCatchTimes = map[uint32]bool{1001: true, 1002: true}
	if p := s.pickLivingBagPet(1, st, bag, 1002); p == nil || p.CatchTime != 1003 {
		t.Fatalf("pick living got %#v", p)
	}
	st.markPetFainted(1003)
	if s.pickLivingBagPet(1, st, bag, 0) != nil {
		t.Fatal("all fainted")
	}
}
