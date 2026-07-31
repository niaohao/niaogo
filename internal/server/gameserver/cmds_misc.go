package gameserver

import (
	"encoding/binary"
	"log"

	"niaohao/server/internal/cmdname"
)

// handleMapHot CMD 1004 宇宙地图热点。
// MapHotInfo: count(4) + [id(4)+value(4)]*count；暂无热点。
func (s *Server) handleMapHot(c *Client, uid uint32, body []byte) {
	if len(body) >= 4 {
		log.Printf("[CMD] OK     1004 MAP_HOT UID=%d index=%d -> count=0", uid, binary.BigEndian.Uint32(body[0:4]))
	} else {
		log.Printf("[CMD] OK     1004 MAP_HOT UID=%d -> count=0", uid)
	}
	out := make([]byte, 4)
	s.send(c, 1004, uid, 0, out)
}

// handleClientReport CMD 1020 客户端行为上报（无监听解析），空 ACK 即可。
func (s *Server) handleClientReport(c *Client, uid uint32, body []byte) {
	sub := uint32(0)
	if len(body) >= 4 {
		sub = binary.BigEndian.Uint32(body[0:4])
	}
	log.Printf("[CMD] OK     1020 CLIENT_REPORT UID=%d sub=%d", uid, sub)
	s.send(c, 1020, uid, 0, nil)
}

// handleFireEdgeCheckEatMedicine CMD 9088：背包面板查询精灵是否吃过烈刃药/雷伊超进化标记。
// 请求 catchTime(4)；应答 2×u32（KTool.readDataByBits 按 u32 数组读）：
//   [0]==1 → 烈刃一族(597/599)显示「帮助烈刃必定繁殖出火刃」
//   [1]==1 → 雷伊(70)显示「电系技能伤害提升」
// 本服暂无对应服药落库，回 0/0（不显示图标，面板不卡死）。
func (s *Server) handleFireEdgeCheckEatMedicine(c *Client, uid uint32, body []byte) {
	catch := uint32(0)
	if len(body) >= 4 {
		catch = binary.BigEndian.Uint32(body[0:4])
	}
	out := make([]byte, 8)
	s.send(c, 9088, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d catch=%d flags=0,0", cmdname.Format(9088), uid, catch)
}

// handlePeopleWalk CMD 2101 人物走路。
// 对齐反编译 WalkCmdListener / WalkAction：
// 请求 walkType(4)+x(4)+y(4)+amfLen(4)+amfData
// 响应 walkType(4)+userID(4)+x(4)+y(4)+amfLen(4)+amfData
// 空包体会使 param1.data==null → onWalk NPE (#1009)。
func (s *Server) handlePeopleWalk(c *Client, uid uint32, body []byte) {
	var walkType, x, y, amfLen uint32
	var amf []byte
	if len(body) >= 12 {
		walkType = binary.BigEndian.Uint32(body[0:4])
		x = binary.BigEndian.Uint32(body[4:8])
		y = binary.BigEndian.Uint32(body[8:12])
	}
	if len(body) >= 16 {
		amfLen = binary.BigEndian.Uint32(body[12:16])
		if amfLen > 0 && len(body) >= 16+int(amfLen) {
			amf = body[16 : 16+int(amfLen)]
			amfLen = uint32(len(amf))
		} else {
			amfLen = 0
			amf = nil
		}
	}
	c.PosX, c.PosY = int(x), int(y)

	out := make([]byte, 20+len(amf))
	binary.BigEndian.PutUint32(out[0:4], walkType)
	binary.BigEndian.PutUint32(out[4:8], uid)
	binary.BigEndian.PutUint32(out[8:12], x)
	binary.BigEndian.PutUint32(out[12:16], y)
	binary.BigEndian.PutUint32(out[16:20], amfLen)
	if len(amf) > 0 {
		copy(out[20:], amf)
	}
	s.send(c, 2101, uid, 0, out)
	s.broadcastToMap(c, 2101, out)
	log.Printf("[CMD] OK     2101 PEOPLE_WALK UID=%d type=%d (%d,%d) amf=%d", uid, walkType, x, y, amfLen)
}
