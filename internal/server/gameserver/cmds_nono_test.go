package gameserver

import (
	"encoding/binary"
	"testing"

	"niaohao/server/internal/store"
)

func TestBuildNonoInfo90Layout(t *testing.T) {
	n := &store.Nono{
		UserID: 10001, HasNono: 1, Flag: 1, State: store.NonoStateFollowBit,
		Nick: "小蓝", Color: 0xFFFFFF, SuperNono: 2, Power: 80, Mate: 90,
		Birth: 1700000000, SuperLevel: 4, SuperStage: 2,
	}
	b := buildNonoInfo90From(n)
	if len(b) != 90 {
		t.Fatalf("len=%d want 90", len(b))
	}
	if binary.BigEndian.Uint32(b[0:4]) != 10001 {
		t.Fatal("userID")
	}
	if binary.BigEndian.Uint32(b[28:32]) != 2 {
		t.Fatal("superNono")
	}
	if binary.BigEndian.Uint32(b[36:40]) != 80000 { // power*1000
		t.Fatalf("power=%d", binary.BigEndian.Uint32(b[36:40]))
	}
	if binary.BigEndian.Uint32(b[82:86]) != 4 {
		t.Fatal("superLevel")
	}
	if binary.BigEndian.Uint32(b[86:90]) != 5 {
		t.Fatal("superStage") // 飞行全面开启：包内至少 stage 5
	}
	// 超能时 func 全 FF
	for i := 58; i < 78; i++ {
		if b[i] != 0xFF {
			t.Fatalf("func[%d]=%d", i-58, b[i])
		}
	}
}

func TestBuildNonoInfo90FlyUnlockWithoutPaidSuper(t *testing.T) {
	n := &store.Nono{UserID: 2, HasNono: 1, Flag: 1, Nick: "蓝", SuperNono: 0, SuperStage: 0}
	b := buildNonoInfo90From(n)
	if binary.BigEndian.Uint32(b[28:32]) != 1 {
		t.Fatal("superNono packet unlock")
	}
	if binary.BigEndian.Uint32(b[86:90]) != 5 {
		t.Fatal("superStage packet unlock")
	}
	for i := 58; i < 78; i++ {
		if b[i] != 0xFF {
			t.Fatalf("funcBits[%d]", i-58)
		}
	}
}

func TestSuperStageByLevel(t *testing.T) {
	cases := []struct{ lv, want int }{
		{0, 0}, {1, 1}, {3, 1}, {4, 2}, {6, 2}, {7, 3}, {8, 3}, {9, 4}, {11, 4}, {12, 5}, {15, 5},
	}
	for _, tc := range cases {
		if got := superStageByLevel(tc.lv); got != tc.want {
			t.Fatalf("lv=%d got=%d want=%d", tc.lv, got, tc.want)
		}
	}
}

func TestNonoFollowBodyLayout(t *testing.T) {
	out := make([]byte, 36)
	binary.BigEndian.PutUint32(out[0:4], 10001)
	binary.BigEndian.PutUint32(out[4:8], 3) // superStage
	binary.BigEndian.PutUint32(out[8:12], 1)
	putFixedNick(out, 12, "NoNo")
	binary.BigEndian.PutUint32(out[28:32], 0xFFFFFF)
	binary.BigEndian.PutUint32(out[32:36], 100000)
	if binary.BigEndian.Uint32(out[4:8]) != 3 {
		t.Fatal("stage")
	}
}

func TestApplySuperNonoExpiry(t *testing.T) {
	s := &Server{}
	n := &store.Nono{UserID: 1, SuperLevel: 5, SuperMonths: 5, SuperNono: 2, SuperStage: 2, VipEndTime: 1}
	s.applySuperNonoExpiry(n)
	if n.SuperLevel != 0 || n.SuperMonths != 5 || n.SuperNono != 0 {
		t.Fatalf("after expiry: %+v", n)
	}
}
