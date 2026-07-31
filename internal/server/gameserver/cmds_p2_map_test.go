package gameserver

import (
	"encoding/binary"
	"net"
	"testing"

	"niaohao/server/internal/store"
)

func TestMap61WeekdayBoss(t *testing.T) {
	for day := uint32(0); day <= 6; day++ {
		pid, lv, name := resolveChallengeBoss(61, day)
		if pid != 421 || lv != 80 || name == "" {
			t.Fatalf("day=%d pet=%d lv=%d name=%q", day, pid, lv, name)
		}
	}
	pid, _, name := resolveChallengeBoss(61, 7)
	if pid != 421 {
		t.Fatalf("train pet=%d name=%q", pid, name)
	}
}

func TestNiguDewTalkCate(t *testing.T) {
	st, err := store.OpenJSON(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.CreateUser("nigu@test.local", "pass")
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
	binary.BigEndian.PutUint32(body, talkCateNiguDewVIP)
	go s.handleTalkCate(c, uint32(uid), body)
	_, out := readOnePkt(t, cli)
	if binary.BigEndian.Uint32(out[4:8]) != 1 {
		t.Fatalf("outCount=%d", binary.BigEndian.Uint32(out[4:8]))
	}
	if binary.BigEndian.Uint32(out[8:12]) != talkNiguDewItem || binary.BigEndian.Uint32(out[12:16]) != 2 {
		t.Fatalf("reward=%x", out[8:16])
	}

	binary.BigEndian.PutUint32(body, talkCateNiguDewVIP)
	go s.handleTalkCount(c, uint32(uid), body)
	_, out2 := readOnePkt(t, cli)
	if binary.BigEndian.Uint32(out2) != 1 {
		t.Fatalf("count=%d", binary.BigEndian.Uint32(out2))
	}
}

func TestPetBargeListOwns239(t *testing.T) {
	st, err := store.OpenJSON(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.CreateUser("barge@test.local", "pass")
	if err != nil || u == nil {
		t.Fatal(err)
	}
	uid := u.UserID
	if _, err := st.GrantPet(uid, 239, "t", 1, 0, 0, nil); err != nil {
		t.Fatal(err)
	}
	s := &Server{cfg: Config{Store: st}, byUID: make(map[int64]*Client)}
	srv, cli := net.Pipe()
	defer srv.Close()
	defer cli.Close()
	c := &Client{Conn: srv, UserID: uid, LoggedIn: true}

	body := make([]byte, 8)
	binary.BigEndian.PutUint32(body[0:4], 239)
	binary.BigEndian.PutUint32(body[4:8], 239)
	go s.handlePetBargeList(c, uint32(uid), body)
	cmd, out := readOnePkt(t, cli)
	if cmd != 2309 {
		t.Fatalf("cmd=%d", cmd)
	}
	if binary.BigEndian.Uint32(out[0:4]) != 1 {
		t.Fatalf("monCount=%d", binary.BigEndian.Uint32(out[0:4]))
	}
	if binary.BigEndian.Uint32(out[4:8]) != 239 {
		t.Fatalf("pet=%d", binary.BigEndian.Uint32(out[4:8]))
	}
	if binary.BigEndian.Uint32(out[16:20]) != 1 {
		t.Fatalf("isKilled=%d", binary.BigEndian.Uint32(out[16:20]))
	}
}
