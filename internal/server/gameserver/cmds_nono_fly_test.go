package gameserver

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"niaohao/server/internal/packet"
	"niaohao/server/internal/store"
)

func TestHandleOnOrOffFlying(t *testing.T) {
	st, err := store.OpenJSON(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.CreateUser("fly@test.local", "pass")
	if err != nil || u == nil {
		t.Fatal(err)
	}
	uid := u.UserID
	n, err := st.GetOrInitNono(uid)
	if err != nil || n == nil {
		t.Fatal(err)
	}
	n.HasNono = 1
	n.Flag = 1
	if err := st.UpsertNono(n); err != nil {
		t.Fatal(err)
	}

	s := &Server{cfg: Config{Store: st}, mapUsers: make(map[int]map[int64]*Client), byUID: make(map[int64]*Client)}
	srv, cli := net.Pipe()
	defer srv.Close()
	defer cli.Close()
	c := &Client{Conn: srv, UserID: uid, LoggedIn: true, MapID: 1}
	s.mapUsers[1] = map[int64]*Client{uid: c}

	type pkt struct {
		cmd  int32
		body []byte
		err  error
	}
	readCh := make(chan pkt, 1)
	startRead := func() {
		go func() {
			_ = cli.SetReadDeadline(time.Now().Add(2 * time.Second))
			hdr := make([]byte, 4)
			if _, err := cli.Read(hdr); err != nil {
				readCh <- pkt{err: err}
				return
			}
			total := int(binary.BigEndian.Uint32(hdr))
			rest := make([]byte, total-4)
			if _, err := cli.Read(rest); err != nil {
				readCh <- pkt{err: err}
				return
			}
			_, cmd, _, _, body, err := packet.ParseHeader(append(hdr, rest...))
			readCh <- pkt{cmd: cmd, body: body, err: err}
		}()
	}

	body := make([]byte, 4)
	binary.BigEndian.PutUint32(body, 9) // clamp → 4
	startRead()
	s.handleOnOrOffFlying(c, uint32(uid), body)
	if c.actionTypeLocked() != 4 {
		t.Fatalf("ActionType=%d want 4", c.actionTypeLocked())
	}
	got := <-readCh
	if got.err != nil {
		t.Fatal(got.err)
	}
	if got.cmd != 2112 || len(got.body) != 8 {
		t.Fatalf("cmd=%d out=%d", got.cmd, len(got.body))
	}
	if binary.BigEndian.Uint32(got.body[0:4]) != uint32(uid) || binary.BigEndian.Uint32(got.body[4:8]) != 4 {
		t.Fatalf("out=%x", got.body)
	}

	n.HasNono = 0
	if err := st.UpsertNono(n); err != nil {
		t.Fatal(err)
	}
	binary.BigEndian.PutUint32(body, 2)
	startRead()
	s.handleOnOrOffFlying(c, uint32(uid), body)
	if c.actionTypeLocked() != 0 {
		t.Fatalf("without nono ActionType=%d", c.actionTypeLocked())
	}
	got = <-readCh
	if got.err != nil {
		t.Fatal(got.err)
	}
	if binary.BigEndian.Uint32(got.body[4:8]) != 0 {
		t.Fatalf("expect land, got %x", got.body)
	}
}
