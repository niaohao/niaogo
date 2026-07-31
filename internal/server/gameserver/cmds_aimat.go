package gameserver

import (
	"encoding/binary"
	"log"

	"niaohao/server/internal/cmdname"
)

// handleAimat CMD 2104 头部射击（AIMAT）。
// 请求 itemID(4)+type(4)+x(4)+y(4)；应答 userID(4)+itemID(4)+type(4)+x(4)+y(4)。
// 空回包会导致 AimatCmdListener 读包失败，射击动画不播放。
func (s *Server) handleAimat(c *Client, uid uint32, body []byte) {
	itemID, aimType, x, y := uint32(0), uint32(0), uint32(0), uint32(0)
	if len(body) >= 16 {
		itemID = binary.BigEndian.Uint32(body[0:4])
		aimType = binary.BigEndian.Uint32(body[4:8])
		x = binary.BigEndian.Uint32(body[8:12])
		y = binary.BigEndian.Uint32(body[12:16])
	}
	out := make([]byte, 20)
	binary.BigEndian.PutUint32(out[0:4], uid)
	binary.BigEndian.PutUint32(out[4:8], itemID)
	binary.BigEndian.PutUint32(out[8:12], aimType)
	binary.BigEndian.PutUint32(out[12:16], x)
	binary.BigEndian.PutUint32(out[16:20], y)
	s.send(c, 2104, uid, 0, out)
	s.broadcastToMap(c, 2104, out)
	log.Printf("[CMD] OK     %s UID=%d item=%d type=%d pos=(%d,%d)",
		cmdname.Format(2104), uid, itemID, aimType, x, y)
}
