package gameserver

import (
	"encoding/binary"
	"log"

	"niaohao/server/internal/cmdname"
)

// ---------- 称号 ----------

func (s *Server) handleAchieveTitleList(c *Client, uid uint32) {
	ids := []int{}
	if s.cfg.Store != nil {
		ids, _ = s.cfg.Store.ListTitles(int64(uid))
	}
	if ids == nil {
		ids = []int{}
	}
	out := make([]byte, 4+4*len(ids))
	binary.BigEndian.PutUint32(out[0:4], uint32(len(ids)))
	for i, id := range ids {
		binary.BigEndian.PutUint32(out[4+4*i:8+4*i], uint32(id))
	}
	s.send(c, 3403, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d count=%d", cmdname.Format(3403), uid, len(ids))
}

func (s *Server) handleSetTitle(c *Client, uid uint32, body []byte) {
	titleID := uint32(0)
	if len(body) >= 4 {
		titleID = binary.BigEndian.Uint32(body[0:4])
	}
	out := make([]byte, 8)
	binary.BigEndian.PutUint32(out[0:4], uid)
	binary.BigEndian.PutUint32(out[4:8], titleID)
	s.send(c, 3404, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d title=%d", cmdname.Format(3404), uid, titleID)
}

// ---------- 米币 ----------

func (s *Server) handleMoneyCheckPsw(c *Client, uid uint32) {
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, 1)
	s.send(c, 1101, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d ok=1", cmdname.Format(1101), uid)
}

func (s *Server) handleMoneyCheckRemain(c *Client, uid uint32) {
	gold := 0
	if s.cfg.Store != nil {
		if u, err := s.cfg.Store.FindByUserID(int64(uid)); err == nil && u != nil {
			gold = u.Gold
		}
	}
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, uint32(gold*100))
	s.send(c, 1103, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d gold=%d", cmdname.Format(1103), uid, gold)
}

// ---------- 好友 INFORM ----------

// pushFriendAddInform 向目标推 8001 InformInfo（type=2151）。
func (s *Server) pushFriendAddInform(fromUID uint32, fromNick string, toUID int64) {
	s.pushInform(toUID, 2151, fromUID, fromNick, 0)
}
