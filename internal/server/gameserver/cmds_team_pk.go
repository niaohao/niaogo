package gameserver

import (
	"crypto/rand"
	"encoding/binary"
	"log"
	"sync"
	"time"

	"niaohao/server/internal/cmdname"
)

// TEAM_PK 进场最小可玩：4001 真 sign+IP/port → 房间图 → 4002 后推 4007 START/OPEN_DOOR；
// 4003/4011/4012 正确包长。不做真实射击对战。

const (
	teamPKEventStart    = 1
	teamPKEventOpenDoor = 2
	teamPKEventOver     = 3
	teamPKSignLen       = 24
	teamPKHQBuildingID  = 1
)

type teamPKSession struct {
	sign     [teamPKSignLen]byte
	homeTeam uint32
	awayTeam uint32
	uid      uint32
	started  bool
}

type teamPKHub struct {
	mu   sync.Mutex
	byUID map[uint32]*teamPKSession
}

func (s *Server) teamPKOf(uid uint32) *teamPKSession {
	s.teamPK.mu.Lock()
	defer s.teamPK.mu.Unlock()
	if s.teamPK.byUID == nil {
		s.teamPK.byUID = map[uint32]*teamPKSession{}
	}
	return s.teamPK.byUID[uid]
}

func (s *Server) teamPKSet(uid uint32, st *teamPKSession) {
	s.teamPK.mu.Lock()
	defer s.teamPK.mu.Unlock()
	if s.teamPK.byUID == nil {
		s.teamPK.byUID = map[uint32]*teamPKSession{}
	}
	if st == nil {
		delete(s.teamPK.byUID, uid)
		return
	}
	s.teamPK.byUID[uid] = st
}

func (s *Server) teamPKHomeTeam(uid uint32) uint32 {
	s.initTeamHub()
	s.teams.mu.Lock()
	tid := s.teams.uidIndex[int64(uid)]
	s.teams.mu.Unlock()
	if tid == 0 {
		// 无战队时用伪 ID，仍可进练习房
		tid = teamMinID + uid%1000
		if tid < teamMinID {
			tid = teamMinID
		}
	}
	return tid
}

// handleTeamPKSign CMD 4001：TeamPKSignInfo = 24B sign + ip(u32) + port(u16)。
func (s *Server) handleTeamPKSign(c *Client, uid uint32, body []byte) {
	home := s.teamPKHomeTeam(uid)
	away := home + 1
	if away < teamMinID {
		away = home + 1
	}
	st := &teamPKSession{homeTeam: home, awayTeam: away, uid: uid}
	_, _ = rand.Read(st.sign[:])
	s.teamPKSet(uid, st)

	out := make([]byte, 30)
	copy(out[0:24], st.sign[:])
	binary.BigEndian.PutUint32(out[24:28], s.advertiseIPUint32())
	binary.BigEndian.PutUint16(out[28:30], uint16(s.cfg.Port))
	s.send(c, 4001, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d home=%d away=%d", cmdname.Format(4001), uid, home, away)
}

// handleTeamPKRegister CMD 4002：校验 sign；空 ACK；推 4007 START 再 OPEN_DOOR。
func (s *Server) handleTeamPKRegister(c *Client, uid uint32, body []byte) {
	st := s.teamPKOf(uid)
	if st == nil {
		st = &teamPKSession{homeTeam: s.teamPKHomeTeam(uid), awayTeam: s.teamPKHomeTeam(uid) + 1, uid: uid}
		_, _ = rand.Read(st.sign[:])
		s.teamPKSet(uid, st)
	}
	if len(body) >= teamPKSignLen {
		var got [teamPKSignLen]byte
		copy(got[:], body[:teamPKSignLen])
		if got != st.sign {
			// 仍放行练习，避免客户端卡死
			log.Printf("[CMD] WARN  %s UID=%d sign mismatch (practice continue)", cmdname.Format(4002), uid)
		}
	}
	s.send(c, 4002, uid, 0, nil)
	st.started = true
	s.pushTeamPKNote(c, uid, st, teamPKEventStart)
	s.pushTeamPKNote(c, uid, st, teamPKEventOpenDoor)
	log.Printf("[CMD] OK     %s UID=%d -> 4007 START+OPEN_DOOR", cmdname.Format(4002), uid)
}

func (s *Server) pushTeamPKNote(c *Client, uid uint32, st *teamPKSession, event uint32) {
	if st == nil {
		return
	}
	out := make([]byte, 20)
	binary.BigEndian.PutUint32(out[0:4], st.homeTeam) // self=home 练习
	binary.BigEndian.PutUint32(out[4:8], st.homeTeam)
	binary.BigEndian.PutUint32(out[8:12], st.awayTeam)
	binary.BigEndian.PutUint32(out[12:16], event)
	binary.BigEndian.PutUint32(out[16:20], uint32(time.Now().Unix()))
	s.send(c, 4007, uid, 0, out)
	log.Printf("[CMD] OK     4007 TEAM_PK_NOTE UID=%d event=%d home=%d away=%d", uid, event, st.homeTeam, st.awayTeam)
}

// handleTeamPKJoin CMD 4003：JoinInfo = homeID+cnt+users + awayID+cnt+users。
func (s *Server) handleTeamPKJoin(c *Client, uid uint32, body []byte) {
	st := s.teamPKOf(uid)
	home, away := s.teamPKHomeTeam(uid), s.teamPKHomeTeam(uid)+1
	if st != nil {
		home, away = st.homeTeam, st.awayTeam
	}
	// 单人练习：home 1 人，away 0
	out := make([]byte, 8+24+8)
	binary.BigEndian.PutUint32(out[0:4], home)
	binary.BigEndian.PutUint32(out[4:8], 1)
	binary.BigEndian.PutUint32(out[8:12], uid)
	binary.BigEndian.PutUint32(out[12:16], 100) // hp
	binary.BigEndian.PutUint32(out[16:20], 100) // maxHp
	// where=0, pad, pad
	binary.BigEndian.PutUint32(out[32:36], away)
	binary.BigEndian.PutUint32(out[36:40], 0)
	s.send(c, 4003, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d home=%d", cmdname.Format(4003), uid, home)
}

// handleTeamPKBuilding CMD 4011：两侧各 1 座 HQ（id=1）。
func (s *Server) handleTeamPKBuilding(c *Client, uid uint32, body []byte) {
	st := s.teamPKOf(uid)
	homeHead := uint32(700001)
	awayHead := uint32(700001)
	putBld := func(dst []byte, id, form, buy, hp, x, y, dir, status uint32) {
		binary.BigEndian.PutUint32(dst[0:4], id)
		binary.BigEndian.PutUint32(dst[4:8], form)
		binary.BigEndian.PutUint32(dst[8:12], buy)
		binary.BigEndian.PutUint32(dst[12:16], hp)
		binary.BigEndian.PutUint32(dst[16:20], x)
		binary.BigEndian.PutUint32(dst[20:24], y)
		binary.BigEndian.PutUint32(dst[24:28], dir)
		binary.BigEndian.PutUint32(dst[28:32], status)
	}
	out := make([]byte, 8+32+8+32)
	binary.BigEndian.PutUint32(out[0:4], 1) // homeCount
	binary.BigEndian.PutUint32(out[4:8], homeHead)
	putBld(out[8:40], teamPKHQBuildingID, 1, 1, 1000, 400, 300, 0, 0)
	binary.BigEndian.PutUint32(out[40:44], 1) // awayCount
	binary.BigEndian.PutUint32(out[44:48], awayHead)
	putBld(out[48:80], teamPKHQBuildingID, 1, 2, 1000, 2200, 300, 0, 0)
	_ = st
	s.send(c, 4011, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(4011), uid)
}

// handleTeamPKSituation CMD 4012：flag=1 + 时间/状态 + 7+7 统计。
func (s *Server) handleTeamPKSituation(c *Client, uid uint32, body []byte) {
	out := make([]byte, 4+4+4+7*4+7*4)
	binary.BigEndian.PutUint32(out[0:4], 1) // flag
	binary.BigEndian.PutUint32(out[4:8], uint32(time.Now().Unix()))
	binary.BigEndian.PutUint32(out[8:12], 2) // pkStatus 进行中
	// home: playerCount=1, hqrtsHp=1000
	binary.BigEndian.PutUint32(out[12:16], 1)
	binary.BigEndian.PutUint32(out[24:28], 1000)
	// away
	binary.BigEndian.PutUint32(out[40:44], 0)
	binary.BigEndian.PutUint32(out[52:56], 1000)
	s.send(c, 4012, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(4012), uid)
}

// handleTeamPKWeekyScore CMD 4017：TeamPkWeekyHistoryInfo 7×u32 = 28B。
func (s *Server) handleTeamPKWeekyScore(c *Client, uid uint32, body []byte) {
	out := make([]byte, 28)
	s.send(c, 4017, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(4017), uid)
}

// handleTeamPKHistory CMD 4018：TeamPkHistoryInfo 7×u32 + week(i32) = 36B。
func (s *Server) handleTeamPKHistory(c *Client, uid uint32, body []byte) {
	out := make([]byte, 36)
	binary.BigEndian.PutUint32(out[32:36], uint32(time.Now().Unix()))
	s.send(c, 4018, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(4018), uid)
}

// handleTeamPKSomeoneJoin CMD 4019：SomeoneJoinInfo 3×u32 = 12B。
func (s *Server) handleTeamPKSomeoneJoin(c *Client, uid uint32, body []byte) {
	out := make([]byte, 12)
	binary.BigEndian.PutUint32(out[0:4], uid)
	binary.BigEndian.PutUint32(out[4:8], 100)
	binary.BigEndian.PutUint32(out[8:12], 100)
	s.send(c, 4019, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(4019), uid)
}

// handleTeamPKEmptyStub 射击等未做真逻辑：空 ACK。
func (s *Server) handleTeamPKEmptyStub(c *Client, uid uint32, cmd int32) {
	s.send(c, cmd, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d (stub)", cmdname.Format(cmd), uid)
}
