package gameserver

import (
	"encoding/binary"
	"testing"

	"niaohao/server/internal/store"
)

func TestBuildRoweiListBody12(t *testing.T) {
	pets := []store.Pet{{PetID: 58, CatchTime: 1001}}
	body := buildRoweiListBody(pets)
	if len(body) != 4+12 {
		t.Fatalf("len=%d", len(body))
	}
	if binary.BigEndian.Uint32(body[0:4]) != 1 {
		t.Fatal("count")
	}
	if binary.BigEndian.Uint32(body[4:8]) != 58 {
		t.Fatal("petid")
	}
}
