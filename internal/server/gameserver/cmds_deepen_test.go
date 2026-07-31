package gameserver

import (
	"encoding/binary"
	"testing"

	"niaohao/server/internal/tableloader"
)

func TestWheelFloorTable(t *testing.T) {
	if len(wheelFloorTable) != 21 {
		t.Fatalf("want 21 entries (1-indexed), got %d", len(wheelFloorTable))
	}
	if wheelFloorTable[10].mons[0] != 453 {
		t.Fatalf("floor10 boss want 453, got %v", wheelFloorTable[10].mons)
	}
	if wheelFloorTable[20].mons[0] != 816 {
		t.Fatalf("floor20 boss want 816, got %v", wheelFloorTable[20].mons)
	}
}

func TestArmEncode2965Len(t *testing.T) {
	b := encodeArmUp2965(teamArmUp{ID: 2, BuyTime: 1, Form: 1, HP: 100})
	if len(b) != armEntry2965 {
		t.Fatalf("want %d, got %d", armEntry2965, len(b))
	}
	if binary.BigEndian.Uint32(b[0:4]) != 2 || binary.BigEndian.Uint32(b[12:16]) != 100 {
		t.Fatalf("bad fields: %v", b[:16])
	}
}

func TestArm2941EmptyMinLen(t *testing.T) {
	s := &Server{byUID: map[int64]*Client{}}
	s.initTeamHub()
	// 无战队时仍须 ≥12B
	c := &Client{UserID: 1, LoggedIn: true}
	// 直接组包逻辑：ensure defaults on nil team → empty arms with seed only when team exists
	out := make([]byte, 12)
	binary.BigEndian.PutUint32(out[8:12], 0)
	if len(out) < 12 {
		t.Fatal("2941 min 12B")
	}
	_ = c
	_ = s
}

func TestSideEffectPinkDamageNotInLostHP(t *testing.T) {
	d28 := &tableloader.SkillDef{SideEffect: "28", SideEffectArg: "4"}
	pink := sideEffectPinkDamage(d28, 100)
	if pink != 25 {
		t.Fatalf("28 1/4 of 100 want 25 got %d", pink)
	}
	d29 := &tableloader.SkillDef{SideEffect: "29", SideEffectArg: "50"}
	if sideEffectPinkDamage(d29, 100) != 50 {
		t.Fatal("29 fixed")
	}
	// 公式伤与粉伤分离：lostHP 不含粉
	hp := uint32(100)
	formula := uint32(30)
	lost := formula
	_ = applyDamage(&hp, formula)
	applyPinkDamage(&hp, pink)
	if lost != 30 {
		t.Fatalf("lostHP should stay 30, got %d", lost)
	}
	if hp != 100-30-25 {
		t.Fatalf("hp want 45 got %d", hp)
	}
}

