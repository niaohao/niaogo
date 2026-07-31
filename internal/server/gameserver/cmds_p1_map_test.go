package gameserver

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"niaohao/server/internal/store"
)

func TestMiningCateItemIDs(t *testing.T) {
	cases := map[uint32]uint32{
		1: 400001, 10: 400011, 11: 400012, 12: 400016, 9: 400010, 7: 400009,
	}
	for cate, want := range cases {
		if got := miningCateToItemID[cate]; got != want {
			t.Fatalf("cate %d item=%d want %d", cate, got, want)
		}
	}
}

func TestMiningCateNageCrystal(t *testing.T) {
	st, err := store.OpenJSON(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.CreateUser("mine@test.local", "pass")
	if err != nil || u == nil {
		t.Fatal(err)
	}
	uid := u.UserID
	s := &Server{cfg: Config{Store: st}, byUID: make(map[int64]*Client)}
	srv, cli := net.Pipe()
	defer srv.Close()
	defer cli.Close()
	c := &Client{Conn: srv, UserID: uid, LoggedIn: true}

	body := make([]byte, 4)
	binary.BigEndian.PutUint32(body, 10)
	go s.handleTalkCate(c, uint32(uid), body)
	cmd, out := readOnePkt(t, cli)
	if cmd != 2702 || len(out) < 16 {
		t.Fatalf("cmd=%d len=%d", cmd, len(out))
	}
	itemID := binary.BigEndian.Uint32(out[8:12])
	if itemID != 400011 {
		t.Fatalf("item=%d want 400011", itemID)
	}
}

func TestMapBossShieldAttack(t *testing.T) {
	s := &Server{byUID: make(map[int64]*Client)}
	srv, cli := net.Pipe()
	defer srv.Close()
	defer cli.Close()
	c := &Client{Conn: srv, UserID: 50001, LoggedIn: true, MapID: 12}

	go s.pushMapBossOnEnter(c, 12)
	for i := 0; i < 2; i++ {
		cmd, body := readOnePkt(t, cli)
		if cmd != 2021 {
			t.Fatalf("push #%d cmd=%d", i, cmd)
		}
		if len(body) < 20 || binary.BigEndian.Uint32(body[4:8]) != 47 {
			t.Fatalf("body=%x", body)
		}
	}

	go s.handleAttackBoss(c, uint32(c.UserID), make([]byte, 4))
	cmd, out := readOnePkt(t, cli)
	if cmd != 2412 {
		t.Fatalf("cmd=%d", cmd)
	}
	if binary.BigEndian.Uint32(out) != 75 {
		t.Fatalf("hp=%d want 75", binary.BigEndian.Uint32(out))
	}
	// 更新 2021
	cmd2, _ := readOnePkt(t, cli)
	if cmd2 != 2021 {
		t.Fatalf("update cmd=%d", cmd2)
	}
}

func TestMapBossFourHitsBreakShield(t *testing.T) {
	s := &Server{byUID: make(map[int64]*Client)}
	srv, cli := net.Pipe()
	defer srv.Close()
	defer cli.Close()
	c := &Client{Conn: srv, UserID: 50002, LoggedIn: true, MapID: 12}
	s.bossShield.reset(c.UserID, 12, 0, mapBossShieldDefault)

	drain := func() {
		_ = cli.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		buf := make([]byte, 4096)
		_, _ = cli.Read(buf)
	}

	var lastHP uint32 = mapBossShieldDefault
	for i := 0; i < 4; i++ {
		go s.handleAttackBoss(c, uint32(c.UserID), make([]byte, 4))
		cmd, out := readOnePkt(t, cli)
		if cmd != 2412 {
			t.Fatalf("hit %d cmd=%d", i, cmd)
		}
		lastHP = binary.BigEndian.Uint32(out)
		// 可能还有 1～2 条 2021
		time.Sleep(20 * time.Millisecond)
		drain()
	}
	if lastHP != 0 {
		t.Fatalf("after 4 hits hp=%d want 0", lastHP)
	}
}
