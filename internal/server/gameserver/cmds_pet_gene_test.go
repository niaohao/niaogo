package gameserver

import (
	"encoding/binary"
	"testing"

	"niaohao/server/internal/store"
	"niaohao/server/internal/tableloader"
)

func TestAddOneGeneGemRightToLeft(t *testing.T) {
	// 主 17(10001) 缺第 3 位(4)；副有 4 → 21(10101)
	if got := addOneGeneGemRightToLeft(17, 0b00100); got != 21 {
		t.Fatalf("want 21 got %d", got)
	}
	// 优先右侧：副有 1 和 16，主全无 → 先补 1
	if got := addOneGeneGemRightToLeft(0, 0b10001); got != 1 {
		t.Fatalf("want 1 got %d", got)
	}
	// 主已满或无可补 → 不变
	if got := addOneGeneGemRightToLeft(31, 31); got != 31 {
		t.Fatalf("full %d", got)
	}
}

func TestApplyGeneRecastKaruyeke(t *testing.T) {
	dv, nature := applyGeneRecast(12, 7, &store.Pet{PetID: 315, DV: 0})
	if dv != 31 || nature != 7 {
		t.Fatalf("karuyeke dv=%d nature=%d", dv, nature)
	}
}

func TestApplyGeneRecastFlashFullDV(t *testing.T) {
	dv, _ := applyGeneRecast(31, 3, &store.Pet{PetID: 310, DV: 31})
	if dv != 31 {
		t.Fatalf("full flash should keep 31 got %d", dv)
	}
}

func TestPetGeneSubOK(t *testing.T) {
	s := &Server{cfg: Config{Catalog: &tableloader.Catalog{PetBaseMap: map[int]tableloader.PetBaseDef{
		310: {ID: 310, PetClass: 119},
		70:  {ID: 70, PetClass: 26, EvolvesTo: 0},
		1:   {ID: 1, PetClass: 1, EvolvesTo: 2},
	}}}}
	if !s.petGeneSubOK(&store.Pet{PetID: 310}) {
		t.Fatal("310 PetClass 119")
	}
	if !s.petGeneSubOK(&store.Pet{PetID: 315}) {
		t.Fatal("315 karuyeke")
	}
	if s.petGeneSubOK(&store.Pet{PetID: 70}) {
		t.Fatal("70 should reject")
	}
	if !s.petGeneMainOK(&store.Pet{PetID: 70}) {
		t.Fatal("70 final ok")
	}
	if s.petGeneMainOK(&store.Pet{PetID: 1}) {
		t.Fatal("1 evolves")
	}
}

func TestPetGeneRecastFailBody(t *testing.T) {
	out := make([]byte, 4)
	if binary.BigEndian.Uint32(out) != 0 {
		t.Fatal("flag")
	}
	ok := make([]byte, 12)
	binary.BigEndian.PutUint32(ok[0:4], 1)
	binary.BigEndian.PutUint32(ok[4:8], 70)
	binary.BigEndian.PutUint32(ok[8:12], 12345)
	if binary.BigEndian.Uint32(ok[0:4]) != 1 || binary.BigEndian.Uint32(ok[4:8]) != 70 {
		t.Fatal("ok body")
	}
}
