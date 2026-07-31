package gameserver

import (
	"encoding/binary"
	"log"

	"niaohao/server/internal/cmdname"
)

// handleOnOrOffFlying CMD 2112：NoNo 飞行开关。
// 请求 flyMode(4)：0=落地，1–4=飞行形态；应答 userID(4)+flyMode(4)，并同图广播。
func (s *Server) handleOnOrOffFlying(c *Client, uid uint32, body []byte) {
	flyMode := uint32(0)
	if len(body) >= 4 {
		flyMode = binary.BigEndian.Uint32(body[0:4])
	}
	if flyMode > 4 {
		flyMode = 4
	}
	// 全面开启：有 NoNo 即可飞，不校验超能/芯片（客户端仍可能提示，服端放行）
	if flyMode > 0 && s.cfg.Store != nil {
		if n, _ := s.cfg.Store.GetOrInitNono(int64(uid)); n == nil || n.HasNono == 0 {
			flyMode = 0
		}
	}
	c.mu.Lock()
	c.ActionType = flyMode
	c.mu.Unlock()

	out := make([]byte, 8)
	binary.BigEndian.PutUint32(out[0:4], uid)
	binary.BigEndian.PutUint32(out[4:8], flyMode)
	s.send(c, 2112, uid, 0, out)
	s.broadcastToMap(c, 2112, out)
	log.Printf("[CMD] OK     %s UID=%d flyMode=%d", cmdname.Format(2112), uid, flyMode)
}
