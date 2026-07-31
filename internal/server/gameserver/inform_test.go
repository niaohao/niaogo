package gameserver

import (
	"encoding/binary"
	"testing"

	"niaohao/server/internal/store"
)

func TestBuildRelationListBody(t *testing.T) {
	body := buildRelationListBody(
		[]store.FriendEntry{{UserID: 10001, TimePoke: 123}},
		[]store.BlackEntry{{UserID: 20002}},
	)
	if len(body) != 4+8+4+4 {
		t.Fatalf("len=%d", len(body))
	}
	if binary.BigEndian.Uint32(body[0:4]) != 1 {
		t.Fatal("friend count")
	}
	if binary.BigEndian.Uint32(body[4:8]) != 10001 {
		t.Fatal("friend id")
	}
	if binary.BigEndian.Uint32(body[12:16]) != 1 {
		t.Fatal("black count")
	}
}

func TestBuildInformBody(t *testing.T) {
	b := buildInformBody(2151, 7, "尼奥", 0, 1, 0, 0, "")
	if len(b) != 104 || binary.BigEndian.Uint32(b[0:4]) != 2151 {
		t.Fatal("inform")
	}
}
