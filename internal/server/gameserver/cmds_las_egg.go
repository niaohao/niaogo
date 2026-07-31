package gameserver

import (
	"log"

	"niaohao/server/internal/cmdname"
)

// CMD 2608 GET_LAS_EGG：实验室领取里奥斯精元（MapProcess_5.getPetHandler → 400107）。
// 道具 Max=1；终身 1 次。
const (
	lasEggItemID = 400107
	lasEggLifeKey = "lasEgg400107"
)

func (s *Server) handleGetLasEgg(c *Client, uid uint32) {
	if s.cfg.Store == nil {
		s.send(c, 2608, uid, 0, nil)
		return
	}
	if !s.tryMarkLifetime(int64(uid), lasEggLifeKey) {
		s.send(c, 2608, uid, 0, nil)
		log.Printf("[CMD] OK     %s UID=%d already claimed", cmdname.Format(2608), uid)
		return
	}
	if err := s.cfg.Store.AddItem(int64(uid), lasEggItemID, 1); err != nil {
		// 回滚终身标记，允许重试
		st := s.loadUserOps(int64(uid))
		delete(st.Lifetime, lasEggLifeKey)
		s.saveUserOps(int64(uid), st)
		s.send(c, 2608, uid, 1, nil)
		log.Printf("[CMD] WARN  %s UID=%d AddItem: %v", cmdname.Format(2608), uid, err)
		return
	}
	s.send(c, 2608, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d item=%d", cmdname.Format(2608), uid, lasEggItemID)
}
