package gameserver

import (
	"encoding/binary"
	"log"
	"sync"

	"niaohao/server/internal/cmdname"
)

// ---------- 神秘洞穴 2493–2499 ----------

func (s *Server) handleMysteryHoleJoin(c *Client, uid uint32, body []byte) {
	s.send(c, 2493, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(2493), uid)
}

func (s *Server) handleMysteryHolePK(c *Client, uid uint32, body []byte) {
	ptype := uint32(0)
	if len(body) >= 4 {
		ptype = binary.BigEndian.Uint32(body[0:4])
	}
	s.send(c, 2494, uid, 0, nil)
	enemyID, lv := 34, 40
	switch ptype {
	case 32:
		enemyID, lv = 34, 35
	case 33:
		enemyID, lv = 70, 50
	case 34:
		enemyID, lv = 71, 45
	case 35:
		enemyID, lv = 72, 55
	}
	s.beginFightVsEnemy(c, uid, enemyID, lv, false, fightKindNormal)
	log.Printf("[CMD] OK     %s UID=%d type=%d enemy=%d", cmdname.Format(2494), uid, ptype, enemyID)
}

func (s *Server) handleMysteryHoleExit(c *Client, uid uint32, body []byte) {
	s.send(c, 2495, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(2495), uid)
}

func (s *Server) handleMysteryHoleBox(c *Client, uid uint32, body []byte) {
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, 1)
	s.send(c, 2496, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d result=1", cmdname.Format(2496), uid)
}

func (s *Server) handleMysteryHoleFront(c *Client, uid uint32, body []byte) {
	s.cureAllBagPets(int64(uid), 0)
	out := make([]byte, 4)
	s.send(c, 2497, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d result=0", cmdname.Format(2497), uid)
}

func (s *Server) handleMysteryHoleData(c *Client, uid uint32, body []byte) {
	pets := []uint32{34, 70, 71}
	out := make([]byte, 4+4*len(pets))
	binary.BigEndian.PutUint32(out[0:4], uint32(len(pets)))
	for i, id := range pets {
		binary.BigEndian.PutUint32(out[4+4*i:8+4*i], id)
	}
	s.send(c, 2499, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d count=%d", cmdname.Format(2499), uid, len(pets))
}

// ---------- NoNo 派对 9032/9033 ----------

func (s *Server) handleNonoPartyGetExp(c *Client, uid uint32, body []byte) {
	const addExp = 3000
	if s.cfg.Store != nil {
		_, _ = s.cfg.Store.AddExpPool(int64(uid), addExp)
	}
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, addExp)
	s.send(c, 9032, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d +%d", cmdname.Format(9032), uid, addExp)
}

func (s *Server) handleNonoPartyGetItem(c *Client, uid uint32, body []byte) {
	s.send(c, 9033, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(9033), uid)
}

// ---------- 组队邀请 7501/7502（本客户端无 7903，仅 ACK + 简要入队回包） ----------

type groupInviteHub struct {
	mu      sync.Mutex
	pending map[uint32]uint32 // invitee -> inviter
	groups  map[uint32][]uint32
	nextID  uint32
}

func (s *Server) handleInviteJoinGroup(c *Client, uid uint32, body []byte) {
	target := uint32(0)
	if len(body) >= 4 {
		target = binary.BigEndian.Uint32(body[0:4])
	}
	s.groupInv.mu.Lock()
	if s.groupInv.pending == nil {
		s.groupInv.pending = map[uint32]uint32{}
	}
	if target > 0 {
		s.groupInv.pending[target] = uid
	}
	s.groupInv.mu.Unlock()
	s.send(c, 7501, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d target=%d", cmdname.Format(7501), uid, target)
}

func (s *Server) handleReplyJoinGroup(c *Client, uid uint32, body []byte) {
	accept := uint32(0)
	if len(body) >= 8 {
		accept = binary.BigEndian.Uint32(body[4:8])
	} else if len(body) >= 4 {
		accept = binary.BigEndian.Uint32(body[0:4])
	}
	s.groupInv.mu.Lock()
	inviter := s.groupInv.pending[uid]
	delete(s.groupInv.pending, uid)
	var out []byte
	if accept == 1 && inviter > 0 {
		if s.groupInv.groups == nil {
			s.groupInv.groups = map[uint32][]uint32{}
		}
		if s.groupInv.nextID == 0 {
			s.groupInv.nextID = 1
		}
		gid := s.groupInv.nextID
		s.groupInv.nextID++
		members := []uint32{inviter, uid}
		s.groupInv.groups[gid] = members
		out = make([]byte, 10+16+4+1+4*len(members))
		binary.BigEndian.PutUint32(out[0:4], gid)
		putFixedNick(out, 10, "小队")
		binary.BigEndian.PutUint32(out[26:30], inviter)
		out[30] = byte(len(members))
		for i, m := range members {
			binary.BigEndian.PutUint32(out[31+4*i:35+4*i], m)
		}
	}
	s.groupInv.mu.Unlock()
	s.send(c, 7502, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d accept=%d", cmdname.Format(7502), uid, accept)
}

// ---------- TEAM_PK_ACTIVE 4022 ----------

func (s *Server) handleTeamPKActive(c *Client, uid uint32, body []byte) {
	out := make([]byte, 4)
	s.send(c, 4022, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(4022), uid)
}
