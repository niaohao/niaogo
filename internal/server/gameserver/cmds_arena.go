package gameserver

import (
	"encoding/binary"
	"log"
	"sync"

	"niaohao/server/internal/cmdname"
)

// 空间站地图 102 擂台（ArenaController / ArenaInfo）。

type arenaHub struct {
	mu           sync.Mutex
	flag         uint32 // 0 空 / 1 有擂主 / 2 战斗中
	hostID       uint32
	hostNick     string
	hostWins     uint32
	challengerID uint32
}

func (s *Server) arenaBuildInfoLocked() []byte {
	out := make([]byte, 32)
	binary.BigEndian.PutUint32(out[0:4], s.arena.flag)
	binary.BigEndian.PutUint32(out[4:8], s.arena.hostID)
	putFixedNick(out, 8, s.arena.hostNick)
	binary.BigEndian.PutUint32(out[24:28], s.arena.hostWins)
	binary.BigEndian.PutUint32(out[28:32], s.arena.challengerID)
	return out
}

func (s *Server) arenaPushInfo(uids ...uint32) {
	s.arena.mu.Lock()
	body := s.arenaBuildInfoLocked()
	s.arena.mu.Unlock()
	seen := map[uint32]bool{}
	for _, u := range uids {
		if u == 0 || seen[u] {
			continue
		}
		seen[u] = true
		if c := s.clientOf(int64(u)); c != nil {
			s.send(c, 2419, u, 0, body)
		}
	}
}

func (s *Server) arenaClearLocked() {
	s.arena.flag = 0
	s.arena.hostID = 0
	s.arena.hostNick = ""
	s.arena.hostWins = 0
	s.arena.challengerID = 0
}

// handleArenaGetInfo CMD 2419：ArenaInfo 固定 32B。
func (s *Server) handleArenaGetInfo(c *Client, uid uint32, body []byte) {
	s.arena.mu.Lock()
	if s.arena.hostID != 0 && s.clientOf(int64(s.arena.hostID)) == nil {
		s.arenaClearLocked()
	}
	out := s.arenaBuildInfoLocked()
	s.arena.mu.Unlock()
	s.send(c, 2419, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d flag=%d host=%d", cmdname.Format(2419), uid, binary.BigEndian.Uint32(out[0:4]), binary.BigEndian.Uint32(out[4:8]))
}

// handleArenaSetOwenr CMD 2417：占擂。
func (s *Server) handleArenaSetOwenr(c *Client, uid uint32, body []byte) {
	s.arena.mu.Lock()
	if s.arena.flag == 0 {
		s.arena.flag = 1
		s.arena.hostID = uid
		s.arena.hostNick = s.nickOf(uid)
		s.arena.hostWins = 0
		s.arena.challengerID = 0
	}
	info := s.arenaBuildInfoLocked()
	s.arena.mu.Unlock()
	s.send(c, 2417, uid, 0, nil)
	s.send(c, 2419, uid, 0, info)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(2417), uid)
}

// handleArenaFightOwenr CMD 2418：挑战擂主 → 单精灵 PvP。
func (s *Server) handleArenaFightOwenr(c *Client, uid uint32, body []byte) {
	s.arena.mu.Lock()
	if s.arena.flag != 1 || s.arena.hostID == 0 || s.arena.hostID == uid {
		info := s.arenaBuildInfoLocked()
		s.arena.mu.Unlock()
		s.send(c, 2418, uid, 0, nil)
		s.send(c, 2419, uid, 0, info)
		return
	}
	hostUID := s.arena.hostID
	s.arena.flag = 2
	s.arena.challengerID = uid
	info := s.arenaBuildInfoLocked()
	s.arena.mu.Unlock()

	s.send(c, 2418, uid, 0, nil)
	s.arenaPushInfo(hostUID, uid)

	if !s.startPvPMatch(int64(hostUID), int64(uid), pvpModeSingle) {
		s.arena.mu.Lock()
		s.arena.flag = 1
		s.arena.challengerID = 0
		s.arena.mu.Unlock()
		s.arenaPushInfo(hostUID, uid)
		_ = info
	}
	log.Printf("[CMD] OK     %s UID=%d vs host=%d", cmdname.Format(2418), uid, hostUID)
}

// handleArenaUpfight CMD 2420：弃擂。
func (s *Server) handleArenaUpfight(c *Client, uid uint32, body []byte) {
	s.arena.mu.Lock()
	notifyHost, notifyChallenger := s.arena.hostID, s.arena.challengerID
	changed := false
	switch {
	case s.arena.flag == 2 && s.arena.challengerID == uid:
		s.arena.flag = 1
		s.arena.challengerID = 0
		changed = true
	case s.arena.hostID == uid:
		s.arenaClearLocked()
		changed = true
	}
	s.arena.mu.Unlock()
	s.send(c, 2420, uid, 0, nil)
	if changed {
		s.arenaPushInfo(notifyHost, notifyChallenger, uid)
	} else {
		s.arenaPushInfo(uid)
	}
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(2420), uid)
}

// handleArenaOwenrAcce CMD 2422：战后确认擂主。
func (s *Server) handleArenaOwenrAcce(c *Client, uid uint32, body []byte) {
	s.arena.mu.Lock()
	notifyHost, notifyChallenger := s.arena.hostID, s.arena.challengerID
	changed := false
	if s.arena.flag == 2 && (uid == s.arena.hostID || uid == s.arena.challengerID) {
		if uid == s.arena.hostID {
			s.arena.hostWins++
		} else {
			s.arena.hostID = uid
			s.arena.hostNick = s.nickOf(uid)
			s.arena.hostWins = 1
		}
		s.arena.flag = 1
		s.arena.challengerID = 0
		changed = true
	}
	s.arena.mu.Unlock()
	s.send(c, 2422, uid, 0, nil)
	if changed {
		s.arenaPushInfo(notifyHost, notifyChallenger)
	}
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(2422), uid)
}

// handleArenaOwenrOut CMD 2423：擂主离开 / 挑战方掉线。
func (s *Server) handleArenaOwenrOut(c *Client, uid uint32, body []byte) {
	s.arena.mu.Lock()
	shouldNotifyHost := false
	hostUID := s.arena.hostID
	if s.arena.flag == 2 && s.arena.challengerID == uid && s.arena.hostID != 0 {
		s.arenaClearLocked()
		shouldNotifyHost = true
	} else if s.arena.hostID == uid {
		s.arenaClearLocked()
	}
	s.arena.mu.Unlock()
	s.send(c, 2423, uid, 0, nil)
	if shouldNotifyHost {
		if hc := s.clientOf(int64(hostUID)); hc != nil {
			s.send(hc, 2423, hostUID, 0, nil)
		}
	}
	s.arenaPushInfo(hostUID, uid)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(2423), uid)
}
