package gameserver

import (
	"encoding/binary"
	"testing"

	"niaohao/server/internal/store"
)

func TestBuildBreedInfoBodyLen(t *testing.T) {
	st := store.BreedState{
		MalePetID: 450, MaleCatchTime: 1,
		FemalePetID: 447, FemaleCatchTime: 2,
		Intimacy: 1,
	}
	b := buildBreedInfoBody(st, 1)
	if len(b) != 44 {
		t.Fatalf("len=%d", len(b))
	}
	if binary.BigEndian.Uint32(b[0:4]) != 2 {
		t.Fatal("breedState")
	}
	if binary.BigEndian.Uint32(b[16:20]) != 450 {
		t.Fatal("maleID")
	}
	if binary.BigEndian.Uint32(b[24:28]) != 447 {
		t.Fatal("femaleID")
	}
}

func TestSyncBreedHatchReady(t *testing.T) {
	st := store.BreedState{
		EggID: 1, EggCatchTime: 1, HatchState: 1, Intimacy: 5,
	}
	if !syncBreedHatchReady(&st) {
		t.Fatal("should ready")
	}
	if st.HatchState != 2 {
		t.Fatal(st.HatchState)
	}
}
