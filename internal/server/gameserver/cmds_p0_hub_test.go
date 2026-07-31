package gameserver

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"niaohao/server/internal/packet"
	"niaohao/server/internal/store"
)

func readOnePkt(t *testing.T, cli net.Conn) (cmd int32, body []byte) {
	t.Helper()
	_ = cli.SetReadDeadline(time.Now().Add(2 * time.Second))
	hdr := make([]byte, 4)
	if _, err := cli.Read(hdr); err != nil {
		t.Fatal(err)
	}
	total := int(binary.BigEndian.Uint32(hdr))
	rest := make([]byte, total-4)
	if _, err := cli.Read(rest); err != nil {
		t.Fatal(err)
	}
	_, cmd, _, _, body, err := packet.ParseHeader(append(hdr, rest...))
	if err != nil {
		t.Fatal(err)
	}
	return cmd, body
}

func TestTalkCateCaptainWeekly(t *testing.T) {
	st, err := store.OpenJSON(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.CreateUser("captain@test.local", "pass")
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
	binary.BigEndian.PutUint32(body, talkCateCaptainCoins)

	go s.handleTalkCate(c, uint32(uid), body)
	cmd, out := readOnePkt(t, cli)
	if cmd != 2702 {
		t.Fatalf("cmd=%d", cmd)
	}
	if len(out) < 16 {
		t.Fatalf("short out=%d", len(out))
	}
	if binary.BigEndian.Uint32(out[4:8]) != 1 {
		t.Fatalf("outCount=%d want 1", binary.BigEndian.Uint32(out[4:8]))
	}
	if binary.BigEndian.Uint32(out[8:12]) != 1 || binary.BigEndian.Uint32(out[12:16]) != talkCaptainCoinsAmt {
		t.Fatalf("reward=%x", out[8:16])
	}

	// 同周再领：无奖励条目
	go s.handleTalkCate(c, uint32(uid), body)
	_, out2 := readOnePkt(t, cli)
	if binary.BigEndian.Uint32(out2[4:8]) != 0 {
		t.Fatalf("second claim outCount=%d want 0", binary.BigEndian.Uint32(out2[4:8]))
	}

	go s.handleTalkCount(c, uint32(uid), body)
	cmd3, out3 := readOnePkt(t, cli)
	if cmd3 != 2701 || binary.BigEndian.Uint32(out3) != 1 {
		t.Fatalf("count cmd=%d n=%d", cmd3, binary.BigEndian.Uint32(out3))
	}
}

func TestTalkCateBalconyExp(t *testing.T) {
	st, err := store.OpenJSON(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.CreateUser("balcony@test.local", "pass")
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
	binary.BigEndian.PutUint32(body, talkCateBalconyExp)
	go s.handleTalkCate(c, uint32(uid), body)
	_, out := readOnePkt(t, cli)
	if binary.BigEndian.Uint32(out[4:8]) != 1 {
		t.Fatalf("outCount=%d", binary.BigEndian.Uint32(out[4:8]))
	}
	if binary.BigEndian.Uint32(out[8:12]) != 3 || binary.BigEndian.Uint32(out[12:16]) != talkBalconyExpAmt {
		t.Fatalf("reward=%x", out[8:16])
	}
	pool, _ := st.GetExpPool(uid)
	if pool < talkBalconyExpAmt {
		t.Fatalf("exp_pool=%d", pool)
	}
}

func TestGetLasEggOnce(t *testing.T) {
	st, err := store.OpenJSON(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.CreateUser("las@test.local", "pass")
	if err != nil || u == nil {
		t.Fatal(err)
	}
	uid := u.UserID
	s := &Server{cfg: Config{Store: st}, byUID: make(map[int64]*Client)}
	srv, cli := net.Pipe()
	defer srv.Close()
	defer cli.Close()
	c := &Client{Conn: srv, UserID: uid, LoggedIn: true}

	go s.handleGetLasEgg(c, uint32(uid))
	cmd, _ := readOnePkt(t, cli)
	if cmd != 2608 {
		t.Fatalf("cmd=%d", cmd)
	}
	n, _ := st.GetItemCount(uid, lasEggItemID)
	if n != 1 {
		t.Fatalf("item count=%d", n)
	}

	go s.handleGetLasEgg(c, uint32(uid))
	_, _ = readOnePkt(t, cli)
	n2, _ := st.GetItemCount(uid, lasEggItemID)
	if n2 != 1 {
		t.Fatalf("second grant count=%d", n2)
	}
}

func TestMiniGameOverUsesScoreNotGameID(t *testing.T) {
	st, err := store.OpenJSON(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.CreateUser("mini@test.local", "pass")
	if err != nil || u == nil {
		t.Fatal(err)
	}
	uid := u.UserID
	s := &Server{cfg: Config{Store: st}, byUID: make(map[int64]*Client)}
	srv, cli := net.Pipe()
	defer srv.Close()
	defer cli.Close()
	c := &Client{Conn: srv, UserID: uid, LoggedIn: true, LastMiniGameID: 1}

	// Map4：per=80, score=1200 → 应记 best=1200（旧逻辑会把 80 当 gameID、1200 当 score 尚可；
	// 但 per=1200,score=1200 时旧逻辑会把 best 记到错误 game 键）
	body := make([]byte, 8)
	binary.BigEndian.PutUint32(body[0:4], 1200)
	binary.BigEndian.PutUint32(body[4:8], 1200)
	go s.handleMiniGameOver(c, uint32(uid), body)
	cmd, out := readOnePkt(t, cli)
	if cmd != 5002 {
		t.Fatalf("cmd=%d", cmd)
	}
	best := binary.BigEndian.Uint32(out[4:8])
	gameID := binary.BigEndian.Uint32(out[8:12])
	if gameID != 1 {
		t.Fatalf("gameID=%d want 1 (from LastMiniGameID)", gameID)
	}
	if best != 1200 {
		t.Fatalf("best=%d want 1200", best)
	}
}
