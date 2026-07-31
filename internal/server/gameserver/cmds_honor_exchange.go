package gameserver

import (
	"encoding/binary"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"

	"niaohao/server/internal/cmdname"
)

// 荣誉兑换手册（FightExchangePanel / TopFightExchangeXMLInfo）
// 70001：count + N×(exchangeID, 剩余可兑次数) —— 客户端显示「可兑换：num / MaxExchange」
// 70002：扣荣誉发奖；exchangeID=0 仅同步荣誉
// 70003：面板只读第 1 个 u32 为当前荣誉（勿把上限放首位）
//
// 70002 成功体：topHonor+monID+capTime+itemCount+[item]*+mintmarkCount+pad
// 精灵走 monID/capTime；失败用包头 result=720001/730000（面板无 body failCode 分支）

const (
	honorExLifetimeKey = "exch_" // Lifetime[exch_<id>] = 已兑次数

	// 包头 result：SocketImpl 在 result!=0 时只走 ParseSocketError，不进面板成功回调
	honorExResultHonour int32 = 720001 // 「你没有足够的荣誉点」
	honorExResultLimit  int32 = 730000 // 「荣誉点商品兑换已达到上限」
)

type honorExchangeEntry struct {
	ID          int
	NeedHonour  int
	ItemID      int
	Type        int // 1精灵 2装备 3道具/家具
	NeedLevel   int
	MaxExchange int
}

var (
	honorExMu     sync.RWMutex
	honorExByID   map[int]honorExchangeEntry
	honorExOrder  []honorExchangeEntry
	honorExLoaded bool
)

func honorExUsedKey(exchangeID int) string {
	return honorExLifetimeKey + strconv.Itoa(exchangeID)
}

func (s *Server) honorExchangeXMLCandidates() []string {
	var out []string
	if s != nil && s.cfg.DataDir != "" {
		root := filepath.Dir(s.cfg.DataDir) // server/
		out = append(out,
			filepath.Join(root, "tables", "xml", "TopFightExchangeXMLInfo.xml"),
			filepath.Clean(filepath.Join(s.cfg.DataDir, "..", "tables", "xml", "TopFightExchangeXMLInfo.xml")),
		)
	}
	out = append(out,
		filepath.Join("tables", "xml", "TopFightExchangeXMLInfo.xml"),
		filepath.Join("server", "tables", "xml", "TopFightExchangeXMLInfo.xml"),
	)
	return out
}

func (s *Server) loadHonorExchangeTable() {
	honorExMu.Lock()
	defer honorExMu.Unlock()
	if honorExLoaded {
		return
	}
	honorExByID = map[int]honorExchangeEntry{}
	honorExOrder = nil
	honorExLoaded = true

	var raw []byte
	var src string
	for _, p := range s.honorExchangeXMLCandidates() {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			raw, src = b, p
			break
		}
	}
	if len(raw) == 0 {
		log.Printf("[honor-ex] 未找到 TopFightExchangeXMLInfo.xml，兑换表为空")
		return
	}
	if len(raw) >= 3 && raw[0] == 0xEF && raw[1] == 0xBB && raw[2] == 0xBF {
		raw = raw[3:]
	}
	tagRe := regexp.MustCompile(`<Exchange\s+([^/>]+)/>`)
	attrRe := regexp.MustCompile(`(\w+)="([^"]*)"`)
	for _, m := range tagRe.FindAllStringSubmatch(string(raw), -1) {
		attrs := map[string]string{}
		for _, a := range attrRe.FindAllStringSubmatch(m[1], -1) {
			attrs[a[1]] = a[2]
		}
		id, _ := strconv.Atoi(attrs["ID"])
		if id <= 0 {
			continue
		}
		needHonour, _ := strconv.Atoi(attrs["NeedHonour"])
		itemID, _ := strconv.Atoi(attrs["ItemID"])
		exType, _ := strconv.Atoi(attrs["type"])
		needLevel, _ := strconv.Atoi(attrs["NeedLevel"])
		maxEx, _ := strconv.Atoi(attrs["MaxExchange"])
		if maxEx <= 0 {
			maxEx = 1
		}
		e := honorExchangeEntry{
			ID: id, NeedHonour: needHonour, ItemID: itemID,
			Type: exType, NeedLevel: needLevel, MaxExchange: maxEx,
		}
		honorExByID[id] = e
		honorExOrder = append(honorExOrder, e)
	}
	log.Printf("[honor-ex] 已加载 %d 条 (%s)", len(honorExOrder), src)
}

func (s *Server) getHonorExchangeEntry(id int) (honorExchangeEntry, bool) {
	s.loadHonorExchangeTable()
	honorExMu.RLock()
	defer honorExMu.RUnlock()
	e, ok := honorExByID[id]
	return e, ok
}

func (s *Server) listHonorExchangeEntries() []honorExchangeEntry {
	s.loadHonorExchangeTable()
	honorExMu.RLock()
	defer honorExMu.RUnlock()
	out := make([]honorExchangeEntry, len(honorExOrder))
	copy(out, honorExOrder)
	return out
}

func honorExchangeRemain(maxEx, used int) uint32 {
	if maxEx >= 999 {
		return 999
	}
	left := maxEx - used
	if left < 0 {
		left = 0
	}
	return uint32(left)
}

// handleExchangeInfo CMD 70001：剩余可兑次数列表。
// 原先回 count=0 导致手册「可兑换：0 / Max」无法兑换。
func (s *Server) handleExchangeInfo(c *Client, uid uint32) {
	entries := s.listHonorExchangeEntries()
	st := s.loadUserOps(int64(uid))
	body := make([]byte, 4+len(entries)*8)
	binary.BigEndian.PutUint32(body[0:4], uint32(len(entries)))
	off := 4
	for _, e := range entries {
		used := st.Lifetime[honorExUsedKey(e.ID)]
		remain := honorExchangeRemain(e.MaxExchange, used)
		binary.BigEndian.PutUint32(body[off:off+4], uint32(e.ID))
		binary.BigEndian.PutUint32(body[off+4:off+8], remain)
		off += 8
	}
	s.send(c, 70001, uid, 0, body)
	log.Printf("[CMD] OK     %s UID=%d n=%d", cmdname.Format(70001), uid, len(entries))
}

// buildHonorExchangeResponse 70002 成功体。
// 前端 onExchangeHandler：topHonor+monID+capTime+itemCount，再 for (i=0;i<=itemCount;i++) 多读一对；
// 故 mintmarkCount 后再补 1×u32 垫齐 off-by-one，避免 EOF。
// 精灵：monID=petID、capTime=catchTime、itemCount=0（勿把精灵 ID 塞进道具列表）。
// 道具/装备：monID=0、item 列表带 ItemID。
func buildHonorExchangeResponse(honour, monID, capTime uint32, itemID int) []byte {
	itemCount := uint32(0)
	if itemID > 0 && monID == 0 {
		itemCount = 1
	}
	// header(16)+items+mintmarkCount(4)+pad(4)
	size := 16 + int(itemCount)*8 + 8
	buf := make([]byte, size)
	off := 0
	binary.BigEndian.PutUint32(buf[off:], honour)
	off += 4
	binary.BigEndian.PutUint32(buf[off:], monID)
	off += 4
	binary.BigEndian.PutUint32(buf[off:], capTime)
	off += 4
	binary.BigEndian.PutUint32(buf[off:], itemCount)
	off += 4
	if itemCount > 0 {
		binary.BigEndian.PutUint32(buf[off:], uint32(itemID))
		off += 4
		binary.BigEndian.PutUint32(buf[off:], 1)
		off += 4
	}
	binary.BigEndian.PutUint32(buf[off:], 0) // mintmarkCount
	off += 4
	binary.BigEndian.PutUint32(buf[off:], 0) // pad for client i<=itemCount
	return buf
}

// handleExchangeItem CMD 70002：荣誉点兑换。
// 请求 exchangeID(4)；0=仅同步荣誉。
func (s *Server) handleExchangeItem(c *Client, uid uint32, body []byte) {
	honour := uint32(s.getHonor(int64(uid)))
	if len(body) < 4 {
		s.send(c, 70002, uid, 0, buildHonorExchangeResponse(honour, 0, 0, 0))
		return
	}
	exchangeID := int(binary.BigEndian.Uint32(body[0:4]))
	if exchangeID <= 0 {
		s.send(c, 70002, uid, 0, buildHonorExchangeResponse(honour, 0, 0, 0))
		log.Printf("[CMD] OK     %s UID=%d sync honour=%d", cmdname.Format(70002), uid, honour)
		return
	}
	entry, ok := s.getHonorExchangeEntry(exchangeID)
	if !ok || entry.ItemID <= 0 {
		s.send(c, 70002, uid, 0, buildHonorExchangeResponse(honour, 0, 0, 0))
		log.Printf("[CMD] WARN  %s UID=%d unknown id=%d", cmdname.Format(70002), uid, exchangeID)
		return
	}
	st := s.loadUserOps(int64(uid))
	used := st.Lifetime[honorExUsedKey(exchangeID)]
	remain := honorExchangeRemain(entry.MaxExchange, used)
	if remain == 0 {
		// 勿带成功形 body：面板无 failCode 分支，会把 monID=0 当成兑换成功弹窗
		s.send(c, 70002, uid, honorExResultLimit, nil)
		log.Printf("[CMD] FAIL  %s UID=%d id=%d limit", cmdname.Format(70002), uid, exchangeID)
		return
	}
	if entry.NeedHonour > 0 && int(honour) < entry.NeedHonour {
		s.send(c, 70002, uid, honorExResultHonour, nil)
		log.Printf("[CMD] FAIL  %s UID=%d id=%d honour=%d need=%d",
			cmdname.Format(70002), uid, exchangeID, honour, entry.NeedHonour)
		return
	}
	if s.cfg.Store == nil {
		s.send(c, 70002, uid, 0, buildHonorExchangeResponse(honour, 0, 0, 0))
		return
	}

	var monID, capTime uint32
	var outItemID int
	switch entry.Type {
	case 1: // 精灵：面板用 monID+capTime 自己 addStorage/setIn，勿再推 8004（会叠弹窗）
		name := "精灵"
		skills := []int{10019}
		if s.cfg.Catalog != nil {
			if n := s.cfg.Catalog.PetNameOf(entry.ItemID); n != "" {
				name = n
			}
			if moves := s.cfg.Catalog.MovesUpToLevel(entry.ItemID, 1); len(moves) > 0 {
				skills = skills[:0]
				for _, m := range moves {
					skills = append(skills, m.ID)
				}
			}
		}
		ct, err := s.cfg.Store.GrantPet(int64(uid), entry.ItemID, name, 1, 20, 0, skills)
		if err != nil {
			log.Printf("[CMD] WARN  %s UID=%d GrantPet %d: %v", cmdname.Format(70002), uid, entry.ItemID, err)
			s.send(c, 70002, uid, 0, buildHonorExchangeResponse(honour, 0, 0, 0))
			return
		}
		monID = uint32(entry.ItemID)
		capTime = uint32(ct)
	default: // 2装备 / 3道具
		if err := s.cfg.Store.AddItem(int64(uid), entry.ItemID, 1); err != nil {
			log.Printf("[CMD] WARN  %s UID=%d AddItem %d: %v", cmdname.Format(70002), uid, entry.ItemID, err)
			s.send(c, 70002, uid, 0, buildHonorExchangeResponse(honour, 0, 0, 0))
			return
		}
		outItemID = entry.ItemID
		s.send(c, 8004, uid, 0, buildBossMonster8004Body(0, 0, 0, uint32(entry.ItemID), 1))
	}

	if entry.NeedHonour > 0 {
		honour = uint32(s.addHonor(int64(uid), -entry.NeedHonour))
	}
	if entry.MaxExchange < 999 {
		s.bumpLifetime(int64(uid), honorExUsedKey(exchangeID))
	}
	out := buildHonorExchangeResponse(honour, monID, capTime, outItemID)
	s.send(c, 70002, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d id=%d item=%d type=%d mon=%d cap=%d honour=%d",
		cmdname.Format(70002), uid, exchangeID, entry.ItemID, entry.Type, monID, capTime, honour)
}
