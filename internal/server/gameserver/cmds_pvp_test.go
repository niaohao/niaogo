package gameserver

import (
	"encoding/binary"
	"testing"

	"niaohao/server/internal/store"
)

func TestInviteNoteBodyLayout(t *testing.T) {
	b := make([]byte, 24)
	binary.BigEndian.PutUint32(b[0:4], 10001)
	putFixedNick(b, 4, "TestNick")
	binary.BigEndian.PutUint32(b[20:24], 2)
	if binary.BigEndian.Uint32(b[0:4]) != 10001 {
		t.Fatal("uid")
	}
	if binary.BigEndian.Uint32(b[20:24]) != 2 {
		t.Fatal("mode")
	}
}

func TestBattleStateIsPvP(t *testing.T) {
	st := &BattleState{OpponentUID: 0}
	if st.isPvP() {
		t.Fatal("pve")
	}
	st.OpponentUID = 2
	if !st.isPvP() {
		t.Fatal("pvp")
	}
}

func TestPvpActionSlotClear(t *testing.T) {
	st := &BattleState{PvPSubmittedType: pvpActSkill, PvPSubmittedSkillID: 10001, PvPDeferSwitch: true}
	st.pvpClearAction()
	if st.pvpHasAction() || st.PvPSubmittedSkillID != 0 || st.PvPDeferSwitch {
		t.Fatal(st)
	}
}

func TestFightPetInfoClampsHP(t *testing.T) {
	b := buildFightPetInfo(1, 10, "test", 99, 111, 100, 10, 0, [6]int8{})
	if len(b) != 50 {
		t.Fatalf("len=%d", len(b))
	}
	hp := binary.BigEndian.Uint32(b[28:32])
	maxHP := binary.BigEndian.Uint32(b[32:36])
	if hp != 100 || maxHP != 100 {
		t.Fatalf("hp=%d max=%d want clamped 100/100", hp, maxHP)
	}
	ct := binary.BigEndian.Uint32(b[24:28])
	if ct != 99 {
		t.Fatalf("catch=%d", ct)
	}
}

func TestPvPNoteReadyKeepsFullBag(t *testing.T) {
	s := &Server{}
	st1 := &BattleState{
		PlayerPetID: 1, PlayerLevel: 5, PlayerHP: 20, PlayerMaxHP: 20, PlayerCatchTime: 1001,
		PlayerSkills: [][2]uint32{{10001, 20}},
	}
	st2 := &BattleState{
		PlayerPetID: 2, PlayerLevel: 10, PlayerHP: 50, PlayerMaxHP: 50, PlayerCatchTime: 2002,
		PlayerSkills: [][2]uint32{{10002, 20}},
	}
	bag1 := []store.Pet{
		{CatchTime: 1001, PetID: 1, Level: 5},
		{CatchTime: 1002, PetID: 3, Level: 6},
	}
	bag2 := []store.Pet{
		{CatchTime: 2002, PetID: 2, Level: 10},
		{CatchTime: 2003, PetID: 4, Level: 11},
	}
	// 单挑模式也必须带全背包，供 SelectPetPanel/_petInfoMap
	body := s.buildNoteReadyToFightPvP(11, "A", st1, bag1, 22, "B", st2, bag2, pvpModeSingle)
	if len(body) < 4 {
		t.Fatal("short")
	}
	// userCount(4) + FighetUserInfo(24)=uid+nick16+topScore
	off := 4 + 24
	petCount1 := binary.BigEndian.Uint32(body[off : off+4])
	if petCount1 < 2 {
		t.Fatalf("self petCount=%d want >=2 (no single truncate) bodyLen=%d", petCount1, len(body))
	}
}
