package gameserver

import (
	"niaohao/server/internal/store"
)

// 登录后首批 Bean/UI 拉取的协议（空列表/默认值即可让客户端继续）。

func (s *Server) handleRelationList(c *Client, uid uint32) {
	var friends []store.FriendEntry
	var blacks []store.BlackEntry
	if s.cfg.Store != nil {
		friends, _ = s.cfg.Store.ListFriends(int64(uid))
		blacks, _ = s.cfg.Store.ListBlacklist(int64(uid))
	}
	if friends == nil {
		friends = []store.FriendEntry{}
	}
	if blacks == nil {
		blacks = []store.BlackEntry{}
	}
	body := buildRelationListBody(friends, blacks)
	s.send(c, 2150, uid, 0, body)
}

// handleGoldOnlineCheckRemain CMD 1106：gold*100 + coins。
func (s *Server) handleGoldOnlineCheckRemain(c *Client, uid uint32) {
	s.pushGoldBalance1106(c, uid)
}
