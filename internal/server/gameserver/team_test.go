package gameserver

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestTeamCreateGetInfoRoundtrip(t *testing.T) {
	dir := t.TempDir()
	s := &Server{cfg: Config{DataDir: dir}, byUID: map[int64]*Client{}}
	s.initTeamHub()

	c := &Client{UserID: 10001, LoggedIn: true}
	// fake send sink via nil Conn is ok — send may log; use nil carefully
	// Call create with body
	name := make([]byte, 96)
	copy(name[0:16], []byte("测试队"))
	copy(name[16:76], []byte("口号口号"))
	binary.BigEndian.PutUint32(name[76:80], 1)
	binary.BigEndian.PutUint32(name[80:84], 0)
	binary.BigEndian.PutUint16(name[84:86], 1)
	binary.BigEndian.PutUint16(name[86:88], 2)
	binary.BigEndian.PutUint16(name[88:90], 3)
	binary.BigEndian.PutUint16(name[90:92], 4)
	copy(name[92:96], []byte("ABCD"))

	// Direct hub create (avoid needing Conn for send)
	h := s.teams
	h.mu.Lock()
	teamID := h.nextID
	if teamID < teamMinID {
		teamID = teamMinID
	}
	h.nextID = teamID + 1
	n, slogan, interest, joinFlag, bg, icon, color, txt, word := parseTeamCreateBody(name)
	t0 := &teamRuntime{
		ID: teamID, LeaderID: 10001, Name: n, Slogan: slogan,
		Interest: interest, JoinFlag: joinFlag, VisitFlag: 1,
		LogoBg: bg, LogoIcon: icon, LogoColor: color, TxtColor: txt, LogoWord: word,
		Members: map[int64]*teamMember{10001: {UserID: 10001, Priv: 0, IsShow: true}},
	}
	h.teams[teamID] = t0
	h.uidIndex[10001] = teamID
	h.saveLocked()
	h.mu.Unlock()

	body := teamBuildSimpleInfoBody(t0)
	if len(body) != 184 {
		t.Fatalf("simple len %d", len(body))
	}
	if binary.BigEndian.Uint32(body[0:4]) != teamID {
		t.Fatal("team id")
	}
	if binary.BigEndian.Uint32(body[4:8]) != 10001 {
		t.Fatal("leader")
	}
	if binary.BigEndian.Uint32(body[12:16]) != 1 {
		t.Fatal("member count")
	}
	logo := teamBuildLogoInfoBody(teamID, t0)
	if len(logo) != 16 {
		t.Fatal("logo len")
	}
	if binary.BigEndian.Uint16(logo[4:6]) != 1 || binary.BigEndian.Uint16(logo[6:8]) != 2 {
		t.Fatal("logo shorts")
	}

	// reload
	s2 := &Server{cfg: Config{DataDir: dir}, byUID: map[int64]*Client{}}
	s2.initTeamHub()
	if s2.teams.teamIDOf(10001) != teamID {
		t.Fatalf("persist uidIndex got %d", s2.teams.teamIDOf(10001))
	}
	snap := s2.userTeamSnapshot(10001)
	if snap.ID != teamID || snap.Priv != 0 {
		t.Fatalf("snapshot %+v", snap)
	}
	_ = c
	_ = os.WriteFile(filepath.Join(dir, "ok"), []byte("1"), 0o644)
}

func TestParseTeamCreateBody(t *testing.T) {
	body := make([]byte, 96)
	copy(body[0:16], []byte("AlphaTeam"))
	copy(body[16:76], []byte("hello slogan"))
	binary.BigEndian.PutUint32(body[76:80], 3)
	binary.BigEndian.PutUint32(body[80:84], 1)
	binary.BigEndian.PutUint16(body[84:86], 9)
	copy(body[92:96], []byte("XY"))
	n, s, interest, join, bg, _, _, _, word := parseTeamCreateBody(body)
	if n != "AlphaTeam" || interest != 3 || join != 1 || bg != 9 || word != "XY" {
		t.Fatalf("%q %q %d %d %d %q slogan=%q", n, word, interest, join, bg, word, s)
	}
}

func TestTeamLogoInfoLen(t *testing.T) {
	out := teamBuildLogoInfoBody(0, nil)
	if len(out) != 16 {
		t.Fatal(len(out))
	}
}
