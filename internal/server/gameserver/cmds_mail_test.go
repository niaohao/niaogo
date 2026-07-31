package gameserver

import (
	"encoding/binary"
	"testing"

	"niaohao/server/internal/store"
)

func TestMailListBodyLayout(t *testing.T) {
	page := []storeMailEntry{{
		ID: 9, Template: 0, MailTime: 1700000000,
		FromID: 10001, FromNick: "系统", Read: false,
	}}
	out := make([]byte, 8+len(page)*36)
	binary.BigEndian.PutUint32(out[0:4], 1)
	binary.BigEndian.PutUint32(out[4:8], uint32(len(page)))
	off := 8
	for _, m := range page {
		binary.BigEndian.PutUint32(out[off:off+4], uint32(m.ID))
		putFixedNick(out, off+16, m.FromNick)
		off += 36
	}
	if len(out) != 44 || binary.BigEndian.Uint32(out[0:4]) != 1 {
		t.Fatalf("mail list body len=%d", len(out))
	}
}

func TestBuildBossMonster8004Body(t *testing.T) {
	itemOnly := buildBossMonster8004Body(0, 0, 0, 300001, 5)
	if len(itemOnly) != 24 {
		t.Fatalf("item body len=%d", len(itemOnly))
	}
	if binary.BigEndian.Uint32(itemOnly[12:16]) != 1 {
		t.Fatal("itemCount")
	}
	petOnly := buildBossMonster8004Body(0, 1, 12345, 0, 0)
	if len(petOnly) != 16 {
		t.Fatalf("pet body len=%d", len(petOnly))
	}
}

func TestMailRewardHasReward(t *testing.T) {
	if (store.MailReward{}).HasReward() {
		t.Fatal("empty")
	}
	if !(store.MailReward{Coins: 100}).HasReward() {
		t.Fatal("coins")
	}
	if !(store.MailReward{Items: []store.MailItemReward{{ItemID: 1, Count: 1}}}).HasReward() {
		t.Fatal("items")
	}
}

func TestLoginProgressPacket(t *testing.T) {
	s := &Server{}
	// 无 Store 时默认 1
	c, max, fc, fm := s.loginProgress(1)
	if c != 1 || max != 1 || fc != 1 || fm != 1 {
		t.Fatalf("defaults %d %d %d %d", c, max, fc, fm)
	}
	pkt := c - 1 // 客户端 +1
	if pkt != 0 {
		t.Fatal("brave stage packet")
	}
}
