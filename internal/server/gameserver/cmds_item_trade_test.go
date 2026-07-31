package gameserver

import (
	"encoding/binary"
	"testing"

	"niaohao/server/internal/store"
)

func TestBuildUsePetItemOutOfFightInfo(t *testing.T) {
	p := &store.Pet{
		CatchTime: 10001,
		PetID:     1,
		Name:      "布布种子",
		Level:     5,
		DV:        20,
		Skills:    []int{10001, 20001},
	}
	body := buildUsePetItemOutOfFightInfo(p)
	// 战外/背包面板保留 Category=4（20001）
	if len(body) < 104 {
		t.Fatalf("len=%d", len(body))
	}
	if binary.BigEndian.Uint32(body[0:4]) != 10001 {
		t.Fatal("catch")
	}
	if binary.BigEndian.Uint32(body[4:8]) != 1 {
		t.Fatal("id")
	}
}

func TestBuildEatSpecialMedicineBody(t *testing.T) {
	b := buildEatSpecialMedicineBody(0, 1001, 20)
	if len(b) != 4 {
		t.Fatalf("fail body len=%d", len(b))
	}
	b = buildEatSpecialMedicineBody(99, 1001, 20)
	if len(b) != 7 {
		t.Fatalf("ok body len=%d", len(b))
	}
	if binary.BigEndian.Uint32(b[0:4]) != 99 {
		t.Fatal("catch")
	}
	if binary.BigEndian.Uint16(b[4:6]) != 1001 {
		t.Fatal("eff")
	}
	if b[6] != 20 {
		t.Fatal("left")
	}
}

func TestBuildPetInfoWithEnergyBall(t *testing.T) {
	p := &store.Pet{
		CatchTime:           10001,
		PetID:               1,
		Name:                "布布种子",
		Level:               5,
		DV:                  20,
		Skills:              []int{10001, 20001},
		EnergyBallItemID:    300030,
		EnergyBallLeftCount: 20,
		EnergyBallEffectID:  1001,
	}
	b := buildPetInfo(p)
	if len(b) != 162+24 {
		t.Fatalf("len=%d want %d", len(b), 186)
	}
	base := buildPetInfo(&store.Pet{CatchTime: 10001, PetID: 1, Name: "布布种子", Level: 5, DV: 20, Skills: []int{10001, 20001}})
	if len(base) != 162 {
		t.Fatalf("base len=%d", len(base))
	}
	// effectCount 在 skin/gen/cost(12) 之前；无特效时位于 162-14
	effOff := 162 - 12 - 2
	if binary.BigEndian.Uint16(b[effOff:effOff+2]) != 1 {
		t.Fatalf("effectCount=%d", binary.BigEndian.Uint16(b[effOff:effOff+2]))
	}
	if binary.BigEndian.Uint32(b[effOff+2:effOff+6]) != 300030 {
		t.Fatal("itemId")
	}
	if b[effOff+6] != 2 || b[effOff+7] != 20 {
		t.Fatalf("status/left %d %d", b[effOff+6], b[effOff+7])
	}
}

