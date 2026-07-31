package gameserver

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"strings"

	"niaohao/server/internal/cmdname"
	"niaohao/server/internal/store"
)

const (
	defaultRoomStyleID = 500001
	roomSessionLen     = 24
	roomDefaultPosX    = 480
	roomDefaultPosY    = 280
)

// ---------- 基地 / 小屋（官方 CommandID）----------
// 10001 ROOM_LOGIN  房间 TCP 登录
// 10002 GET_ROOM_ADDRES 取房间服地址
// 10003 LEAVE_ROOM
// 10004 BUY_FITMENT
// 10005 BETRAY_FITMENT
// 10006 FITMENT_USERING 已摆放家具
// 10007 FITMENT_ALL 仓库家具
// 10008 SET_FITMENT
// 10009 ADD_ENERGY

func (s *Server) handleGetRoomAddress(c *Client, uid uint32, body []byte) {
	roomID := uid
	if len(body) >= 4 {
		if id := binary.BigEndian.Uint32(body[0:4]); id != 0 {
			roomID = id
		}
	}
	out := make([]byte, roomSessionLen+4+2)
	sess := s.roomSessionBytes(int64(uid))
	copy(out[0:roomSessionLen], sess)
	binary.BigEndian.PutUint32(out[roomSessionLen:roomSessionLen+4], s.advertiseIPUint32())
	binary.BigEndian.PutUint16(out[roomSessionLen+4:roomSessionLen+6], uint16(s.cfg.Port))
	s.send(c, 10002, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d room=%d ip=%s:%d",
		cmdname.Format(10002), uid, roomID, s.advertiseHost(), s.cfg.Port)
}

func (s *Server) handleRoomLogin(c *Client, uid uint32, body []byte) {
	userID := int64(uid)
	if userID == 0 {
		s.send(c, 10001, uid, 1, nil)
		return
	}
	// session(24) + catchTime + mapType + roomID + x + y
	if len(body) < roomSessionLen+20 {
		log.Printf("[CMD] FAIL   %s UID=%d body=%d (need session+coords)", cmdname.Format(10001), uid, len(body))
		s.send(c, 10001, uid, 1, nil)
		return
	}
	if !s.sessionMatch(userID, body[:roomSessionLen]) {
		log.Printf("[CMD] FAIL   %s UID=%d session mismatch", cmdname.Format(10001), uid)
		s.send(c, 10001, uid, 1, nil)
		return
	}
	roomID := int(binary.BigEndian.Uint32(body[roomSessionLen+8 : roomSessionLen+12]))
	x := int(binary.BigEndian.Uint32(body[roomSessionLen+12 : roomSessionLen+16]))
	y := int(binary.BigEndian.Uint32(body[roomSessionLen+16 : roomSessionLen+20]))
	if roomID == 0 {
		roomID = int(userID)
	}
	if x == 0 && y == 0 {
		x, y = roomDefaultPosX, roomDefaultPosY
	}

	s.forceDisconnectRoom(userID)
	c.UserID = userID
	c.LoggedIn = true
	c.IsRoom = true
	c.MapID = roomID
	c.PosX = x
	c.PosY = y
	c.ClothIDs = s.wornClothIDs(userID)

	s.mu.Lock()
	s.roomByUID[userID] = c
	s.mu.Unlock()

	s.send(c, 10001, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d room=%d (%d,%d)", cmdname.Format(10001), uid, roomID, x, y)

	user, _ := s.cfg.Store.FindByUserID(userID)
	if user == nil {
		user = &store.User{UserID: userID, Nickname: fmt.Sprintf("%d", userID)}
	}
	// 主连仍在 byUID；房间图只登记 room 连接，避免踢主连
	s.putOnMap(c, roomID)
	people := s.buildPeopleInfo(user, x, y, c.ClothIDs, c.actionTypeLocked())
	s.send(c, 2001, uid, 0, people)
	s.send(c, 2003, uid, 0, s.buildMapPlayerList(roomID))
	log.Printf("[CMD] OK     %s UID=%d body=%d (房间进图)", cmdname.Format(2001), uid, len(people))
}

func (s *Server) handleLeaveRoom(c *Client, uid uint32, body []byte) {
	// 客户端 RoomController.outRoom：mainSocket 发 [flag, mapID, catchTime, shape, action]
	// ACK 10003 后须在主连推 2001 ENTER_MAP，否则 MapController 不发进图、_isSwitching 卡住（离开基地离不开）。
	// 勿在此强制断开房间 TCP：会触发 onClose→changeMap(1) 与目标图争抢。
	destMap := defaultMapID
	if len(body) >= 8 {
		if m := int(binary.BigEndian.Uint32(body[4:8])); m > 0 {
			destMap = m
		}
	}
	// 目的地若误成基地 UID（>ID_MAX），回落传送舱
	if destMap > 49999 {
		destMap = defaultMapID
	}

	s.send(c, 10003, uid, 0, nil)

	main := c
	if c == nil || c.IsRoom {
		s.mu.Lock()
		main = s.byUID[int64(uid)]
		s.mu.Unlock()
	}
	if main != nil && main.LoggedIn {
		s.enterMapForClient(main, destMap, defaultPosX, defaultPosY)
	}

	// 仅注销房间连接登记，由客户端 close() 关 socket
	s.detachRoomClient(int64(uid))
	log.Printf("[CMD] OK     %s UID=%d destMap=%d (ACK+主连进图)", cmdname.Format(10003), uid, destMap)
}

// detachRoomClient 从 roomByUID/地图表摘掉房间连，不关 TCP。
func (s *Server) detachRoomClient(uid int64) {
	s.mu.Lock()
	rc := s.roomByUID[uid]
	if rc != nil {
		delete(s.roomByUID, uid)
		s.removeFromMapLocked(rc)
	}
	s.mu.Unlock()
}

func (s *Server) handleFitmentUsering(c *Client, uid uint32, body []byte) {
	owner := s.resolveFitmentOwner(uid, body)
	bag := s.loadFitmentBag(int64(owner))
	if int64(owner) == int64(uid) {
		if syncOwnerFitmentsOnEnter(bag) {
			s.persistFitmentBag(int64(owner), bag)
		}
	} else {
		sanitizeUserFitments(bag)
	}
	list := bag.Fitments
	out := make([]byte, 12+len(list)*20)
	binary.BigEndian.PutUint32(out[0:4], owner)
	binary.BigEndian.PutUint32(out[4:8], owner)
	binary.BigEndian.PutUint32(out[8:12], uint32(len(list)))
	off := 12
	for _, f := range list {
		binary.BigEndian.PutUint32(out[off:off+4], uint32(f.ID))
		binary.BigEndian.PutUint32(out[off+4:off+8], uint32(f.X))
		binary.BigEndian.PutUint32(out[off+8:off+12], uint32(f.Y))
		binary.BigEndian.PutUint32(out[off+12:off+16], uint32(f.Dir))
		binary.BigEndian.PutUint32(out[off+16:off+20], uint32(f.Status))
		off += 20
	}
	s.send(c, 10006, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d owner=%d count=%d", cmdname.Format(10006), uid, owner, len(list))
}

func (s *Server) handleFitmentAll(c *Client, uid uint32) {
	bag := s.loadFitmentBag(int64(uid))
	// 装修面板打开时也同步一次，避免仓库空、场景有摆件不一致
	if syncOwnerFitmentsOnEnter(bag) {
		s.persistFitmentBag(int64(uid), bag)
	}
	rows := buildFitmentStorageRows(bag)
	out := make([]byte, 4+len(rows)*12)
	binary.BigEndian.PutUint32(out[0:4], uint32(len(rows)))
	off := 4
	for _, r := range rows {
		binary.BigEndian.PutUint32(out[off:off+4], uint32(r[0]))
		binary.BigEndian.PutUint32(out[off+4:off+8], uint32(r[1]))
		binary.BigEndian.PutUint32(out[off+8:off+12], uint32(r[2]))
		off += 12
	}
	s.send(c, 10007, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d count=%d", cmdname.Format(10007), uid, len(rows))
}

func (s *Server) handleSetFitment(c *Client, uid uint32, body []byte) {
	bag := s.loadFitmentBag(int64(uid))
	before := append([]store.Fitment(nil), bag.Fitments...)
	roomID := uint32(0)
	if len(body) >= 4 {
		roomID = binary.BigEndian.Uint32(body[0:4])
	}
	if len(body) < 8 {
		s.send(c, 10008, uid, 0, nil)
		log.Printf("[CMD] OK     %s UID=%d (短包)", cmdname.Format(10008), uid)
		return
	}
	count := int(binary.BigEndian.Uint32(body[4:8]))
	if count < 0 {
		count = 0
	}
	offset := 8
	fitments := make([]store.Fitment, 0, count)
	for i := 0; i < count; i++ {
		if offset+20 > len(body) {
			break
		}
		id := int(binary.BigEndian.Uint32(body[offset : offset+4]))
		x := int(binary.BigEndian.Uint32(body[offset+4 : offset+8]))
		y := int(binary.BigEndian.Uint32(body[offset+8 : offset+12]))
		dir := int(binary.BigEndian.Uint32(body[offset+12 : offset+16]))
		status := int(binary.BigEndian.Uint32(body[offset+16 : offset+20]))
		offset += 20
		if id <= 0 || !isValidFitmentID(id) {
			continue
		}
		ff := store.Fitment{ID: id, X: x, Y: y, Dir: dir, Status: status}
		clampFitmentFields(&ff)
		fitments = append(fitments, ff)
	}
	if len(fitments) > maxPlacedFitments {
		fitments = fitments[:maxPlacedFitments]
	}

	// 仅换房型：count=1 且为 FRAME → 替换房型保留其它摆件
	if len(fitments) == 1 && fitments[0].ID >= fitmentFrameMinID && fitments[0].ID <= fitmentFrameMaxID {
		newFrame := fitments[0]
		merged := make([]store.Fitment, 0, len(bag.Fitments))
		for _, f := range bag.Fitments {
			if f.ID >= fitmentFrameMinID && f.ID <= fitmentFrameMaxID {
				continue
			}
			merged = append(merged, f)
		}
		merged = append(merged, newFrame)
		bag.Fitments = merged
		log.Printf("[CMD] OK     %s UID=%d frame=%d keep=%d room=%d",
			cmdname.Format(10008), uid, newFrame.ID, len(merged)-1, roomID)
	} else {
		bag.Fitments = fitments
		log.Printf("[CMD] OK     %s UID=%d items=%d room=%d", cmdname.Format(10008), uid, len(fitments), roomID)
	}
	sanitizeUserFitments(bag)
	reconcileFitmentWarehouse(bag, before, bag.Fitments)
	s.persistFitmentBag(int64(uid), bag)
	s.send(c, 10008, uid, 0, nil)
}

func (s *Server) handleBuyFitment(c *Client, uid uint32, body []byte) {
	itemID := 0
	if len(body) >= 4 {
		itemID = int(binary.BigEndian.Uint32(body[0:4]))
	}
	if itemID <= 0 || !isValidFitmentID(itemID) {
		s.sendBuyFitment(c, uid, 0, uint32(itemID), 0)
		log.Printf("[CMD] FAIL   %s UID=%d bad item=%d", cmdname.Format(10004), uid, itemID)
		return
	}
	price := s.fitmentCatalogPrice(itemID)
	if len(body) >= 12 {
		if p := int(binary.BigEndian.Uint32(body[8:12])); p > 0 {
			price = p
		}
	} else if len(body) >= 8 {
		if p := int(binary.BigEndian.Uint32(body[4:8])); p > 0 && p < 1000000 {
			price = p
		}
	}
	coins := 0
	if s.cfg.Store != nil {
		if u, err := s.cfg.Store.FindByUserID(int64(uid)); err == nil && u != nil {
			coins = u.Coins
		}
	}
	if price > 0 {
		if s.cfg.Store == nil {
			s.sendBuyFitment(c, uid, uint32(coins), uint32(itemID), 0)
			return
		}
		bal, ok, err := s.cfg.Store.TrySpendCoins(int64(uid), price)
		if err != nil || !ok {
			s.sendBuyFitment(c, uid, uint32(coins), uint32(itemID), 0)
			log.Printf("[CMD] FAIL   %s UID=%d item=%d price=%d coins=%d", cmdname.Format(10004), uid, itemID, price, coins)
			return
		}
		coins = bal
	}
	owned := 0
	if s.cfg.Store != nil {
		_ = s.cfg.Store.AddItem(int64(uid), itemID, 1)
		owned, _ = s.cfg.Store.GetItemCount(int64(uid), itemID)
	} else {
		owned = 1
	}

	s.sendBuyFitment(c, uid, uint32(coins), uint32(itemID), uint32(owned))
	log.Printf("[CMD] OK     %s UID=%d item=%d price=%d coins=%d owned=%d",
		cmdname.Format(10004), uid, itemID, price, coins, owned)
}

func (s *Server) sendBuyFitment(c *Client, uid, coins, itemID, owned uint32) {
	out := make([]byte, 12)
	binary.BigEndian.PutUint32(out[0:4], coins)
	binary.BigEndian.PutUint32(out[4:8], itemID)
	binary.BigEndian.PutUint32(out[8:12], owned)
	s.send(c, 10004, uid, 0, out)
}

func (s *Server) handleBetrayFitment(c *Client, uid uint32) {
	s.send(c, 10005, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(10005), uid)
}

func (s *Server) handleAddEnergy(c *Client, uid uint32) {
	s.send(c, 10009, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(10009), uid)
}

func (s *Server) resolveFitmentOwner(self uint32, body []byte) uint32 {
	if len(body) < 4 {
		return self
	}
	id := binary.BigEndian.Uint32(body[0:4])
	if id == 0 {
		return self
	}
	// 误传房型/家具 ID（>=500001）时查自己
	if id >= fitmentItemMinID {
		return self
	}
	return id
}

func (s *Server) loadFitmentBag(uid int64) *fitmentBag {
	bag := &fitmentBag{Fitments: []store.Fitment{}, Items: map[int]int{}}
	if s.cfg.Store == nil || uid <= 0 {
		return bag
	}
	if list, err := s.cfg.Store.ListFitments(uid); err == nil && list != nil {
		bag.Fitments = list
	}
	if items, err := s.cfg.Store.ListFitmentItems(uid); err == nil {
		for _, it := range items {
			if it.ItemID > 0 && it.Count > 0 {
				bag.Items[it.ItemID] = it.Count
			}
		}
	}
	return bag
}

// persistFitmentBag 写回摆放表，并用 AddItem 差值同步仓库 items。
func (s *Server) persistFitmentBag(uid int64, bag *fitmentBag) {
	if s.cfg.Store == nil || bag == nil || uid <= 0 {
		return
	}
	oldItems := map[int]int{}
	if items, err := s.cfg.Store.ListFitmentItems(uid); err == nil {
		for _, it := range items {
			oldItems[it.ItemID] = it.Count
		}
	}
	_ = s.cfg.Store.ReplaceFitments(uid, bag.Fitments)
	seen := map[int]struct{}{}
	for id := range oldItems {
		seen[id] = struct{}{}
	}
	for id := range bag.Items {
		seen[id] = struct{}{}
	}
	for id := range seen {
		want := bag.Items[id]
		have := oldItems[id]
		delta := want - have
		if delta == 0 {
			continue
		}
		if delta > 0 {
			_ = s.cfg.Store.AddItem(uid, id, delta)
		} else {
			_ = s.cfg.Store.ConsumeItem(uid, id, -delta)
		}
	}
}

func (s *Server) fitmentCatalogPrice(itemID int) int {
	if s.cfg.Catalog == nil || itemID <= 0 {
		return 0
	}
	return s.cfg.Catalog.FitmentPrice(itemID)
}

func (s *Server) forceDisconnectRoom(uid int64) {
	s.mu.Lock()
	rc := s.roomByUID[uid]
	if rc != nil {
		delete(s.roomByUID, uid)
		s.removeFromMapLocked(rc)
	}
	s.mu.Unlock()
	if rc != nil {
		_ = rc.Conn.Close()
	}
}

func (s *Server) advertiseHost() string {
	h := strings.TrimSpace(s.cfg.AdvertiseHost)
	if h == "" {
		return "127.0.0.1"
	}
	return h
}

func (s *Server) advertiseIPUint32() uint32 {
	host := s.advertiseHost()
	ip := net.ParseIP(host)
	if ip == nil {
		return binary.BigEndian.Uint32([]byte{127, 0, 0, 1})
	}
	v4 := ip.To4()
	if v4 == nil {
		return binary.BigEndian.Uint32([]byte{127, 0, 0, 1})
	}
	return binary.BigEndian.Uint32(v4)
}

func (s *Server) roomSessionBytes(uid int64) []byte {
	out := make([]byte, roomSessionLen)
	if s.cfg.Store == nil {
		return out
	}
	u, err := s.cfg.Store.FindByUserID(uid)
	if err != nil || u == nil || u.SessionHex == "" {
		return out
	}
	raw, err := hex.DecodeString(u.SessionHex)
	if err != nil {
		return out
	}
	n := len(raw)
	if n > roomSessionLen {
		n = roomSessionLen
	}
	copy(out, raw[:n])
	return out
}

func (s *Server) sessionMatch(uid int64, got []byte) bool {
	if s.cfg.Store == nil || len(got) == 0 {
		return true
	}
	u, err := s.cfg.Store.FindByUserID(uid)
	if err != nil || u == nil || u.SessionHex == "" {
		return true
	}
	raw, err := hex.DecodeString(u.SessionHex)
	if err != nil || len(raw) == 0 {
		return true
	}
	n := len(raw)
	if n > len(got) {
		n = len(got)
	}
	return strings.EqualFold(hex.EncodeToString(got[:n]), hex.EncodeToString(raw[:n]))
}
