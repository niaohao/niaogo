package gameserver

import (
	"encoding/binary"
	"log"
	"time"

	"niaohao/server/internal/cmdname"
	"niaohao/server/internal/store"
)

func putFixedNick(dst []byte, off int, nick string) {
	nb := []byte(nick)
	if len(nb) > 16 {
		nb = nb[:16]
	}
	copy(dst[off:off+16], nb)
}

// buildBossMonster8004Body CMD 8004：bonusID+petID+captureTm+itemCount+[itemID+itemCnt]*n
func buildBossMonster8004Body(bonusID, petID, captureTm, itemID, itemCnt uint32) []byte {
	itemCount := uint32(0)
	if itemCnt > 0 {
		itemCount = 1
	}
	body := make([]byte, 16+int(itemCount)*8)
	binary.BigEndian.PutUint32(body[0:4], bonusID)
	binary.BigEndian.PutUint32(body[4:8], petID)
	binary.BigEndian.PutUint32(body[8:12], captureTm)
	binary.BigEndian.PutUint32(body[12:16], itemCount)
	if itemCount > 0 {
		binary.BigEndian.PutUint32(body[16:20], itemID)
		binary.BigEndian.PutUint32(body[20:24], itemCnt)
	}
	return body
}

const (
	mailRewardItemCoins = 1 // 赛尔豆（客户端 8004 展示）
	mailRewardItemGold  = 5 // 黄金豆/钻石
)

// handleMailGetList CMD 2751：total(4)+count(4)+N×SingleMailInfo(36B)。
func (s *Server) handleMailGetList(c *Client, uid uint32, body []byte) {
	start := 1
	if len(body) >= 4 {
		start = int(binary.BigEndian.Uint32(body[0:4]))
	}
	if start < 1 {
		start = 1
	}
	total, page := 0, []storeMailEntry(nil)
	if s.cfg.Store != nil {
		var err error
		total, page, err = s.listMailsFromStore(int64(uid), start)
		if err != nil {
			log.Printf("[CMD] WARN  %s UID=%d list: %v", cmdname.Format(2751), uid, err)
		}
	}
	out := make([]byte, 8+len(page)*36)
	binary.BigEndian.PutUint32(out[0:4], uint32(total))
	binary.BigEndian.PutUint32(out[4:8], uint32(len(page)))
	off := 8
	for _, m := range page {
		binary.BigEndian.PutUint32(out[off:off+4], uint32(m.ID))
		binary.BigEndian.PutUint32(out[off+4:off+8], uint32(m.Template))
		binary.BigEndian.PutUint32(out[off+8:off+12], uint32(m.MailTime))
		binary.BigEndian.PutUint32(out[off+12:off+16], uint32(m.FromID))
		putFixedNick(out, off+16, m.FromNick)
		flag := uint32(0)
		if m.Read {
			flag = 1
		}
		binary.BigEndian.PutUint32(out[off+32:off+36], flag)
		off += 36
	}
	s.send(c, 2751, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d total=%d page=%d", cmdname.Format(2751), uid, total, len(page))
}

type storeMailEntry struct {
	ID       int64
	Template int
	MailTime int64
	FromID   int64
	FromNick string
	Read     bool
}

func (s *Server) listMailsFromStore(uid int64, start int) (int, []storeMailEntry, error) {
	total, mails, err := s.cfg.Store.ListMails(uid, start)
	if err != nil {
		return 0, nil, err
	}
	out := make([]storeMailEntry, 0, len(mails))
	for _, m := range mails {
		out = append(out, storeMailEntry{
			ID: m.ID, Template: m.Template, MailTime: m.MailTime,
			FromID: m.FromID, FromNick: m.FromNick, Read: m.Read,
		})
	}
	return total, out, nil
}

// handleMailSend CMD 2752：toUserID+titleLen+title+contentLen+content；应答 coins(4)。
func (s *Server) handleMailSend(c *Client, uid uint32, body []byte) {
	toID := uint32(0)
	off := 0
	if len(body) >= 4 {
		toID = binary.BigEndian.Uint32(body[0:4])
		off = 4
	}
	readBlock := func() []byte {
		if off+4 > len(body) {
			return nil
		}
		n := int(binary.BigEndian.Uint32(body[off : off+4]))
		off += 4
		if n < 0 || off+n > len(body) {
			return nil
		}
		b := body[off : off+n]
		off += n
		return b
	}
	title := string(readBlock())
	content := string(readBlock())
	if content == "" {
		content = title
	}
	if toID != 0 && toID != uid && s.cfg.Store != nil && content != "" {
		nick := s.nickOf(uid)
		if _, err := s.cfg.Store.InsertMail(int64(toID), 0, int64(uid), nick, content); err != nil {
			log.Printf("[CMD] WARN  %s UID=%d to=%d insert: %v", cmdname.Format(2752), uid, toID, err)
		} else {
			s.sendToUser(int64(toID), 8008, nil)
		}
	}
	coins := 0
	if s.cfg.Store != nil {
		if u, err := s.cfg.Store.FindByUserID(int64(uid)); err == nil && u != nil {
			coins = u.Coins
		}
	}
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, uint32(coins))
	s.send(c, 2752, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d to=%d", cmdname.Format(2752), uid, toID)
}

// handleMailGetContent CMD 2753：id(4)+template+time+from+nick+flag+len+content。
func (s *Server) handleMailGetContent(c *Client, uid uint32, body []byte) {
	mailID := int64(0)
	if len(body) >= 4 {
		mailID = int64(binary.BigEndian.Uint32(body[0:4]))
	}
	if s.cfg.Store == nil || mailID <= 0 {
		s.send(c, 2753, uid, 0, nil)
		return
	}
	m, err := s.cfg.Store.GetMail(int64(uid), mailID)
	if err != nil || m == nil {
		s.send(c, 2753, uid, 0, nil)
		log.Printf("[CMD] OK     %s UID=%d mail=%d (missing)", cmdname.Format(2753), uid, mailID)
		return
	}
	content := []byte(m.Content)
	out := make([]byte, 40+len(content))
	binary.BigEndian.PutUint32(out[0:4], uint32(m.ID))
	binary.BigEndian.PutUint32(out[4:8], uint32(m.Template))
	binary.BigEndian.PutUint32(out[8:12], uint32(m.MailTime))
	binary.BigEndian.PutUint32(out[12:16], uint32(m.FromID))
	putFixedNick(out, 16, m.FromNick)
	flag := uint32(0)
	if m.Read {
		flag = 1
	}
	binary.BigEndian.PutUint32(out[32:36], flag)
	binary.BigEndian.PutUint32(out[36:40], uint32(len(content)))
	copy(out[40:], content)
	s.send(c, 2753, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d mail=%d len=%d", cmdname.Format(2753), uid, mailID, len(content))
}

// handleMailSetReaded CMD 2754：count+N×mailID；首次已读时发放附件并推 8004。
func (s *Server) handleMailSetReaded(c *Client, uid uint32, body []byte) {
	n := 0
	off := 0
	if len(body) >= 4 {
		n = int(binary.BigEndian.Uint32(body[0:4]))
		off = 4
	}
	ids := make([]int64, 0, n)
	for i := 0; i < n && off+4 <= len(body); i++ {
		ids = append(ids, int64(binary.BigEndian.Uint32(body[off:off+4])))
		off += 4
	}
	granted := 0
	if s.cfg.Store != nil && len(ids) > 0 {
		claimed, err := s.cfg.Store.MarkMailsReadAndClaim(int64(uid), ids)
		if err != nil {
			log.Printf("[CMD] WARN  %s UID=%d claim: %v", cmdname.Format(2754), uid, err)
		}
		for _, m := range claimed {
			s.grantMailRewards(c, uid, m.Reward)
			granted++
		}
	}
	s.send(c, 2754, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d n=%d granted=%d", cmdname.Format(2754), uid, len(ids), granted)
}

func (s *Server) grantMailRewards(c *Client, uid uint32, reward store.MailReward) {
	if s.cfg.Store == nil || !reward.HasReward() {
		return
	}
	uid64 := int64(uid)
	if reward.Coins > 0 {
		if err := s.cfg.Store.AddCoins(uid64, reward.Coins); err != nil {
			log.Printf("[mail] AddCoins uid=%d: %v", uid, err)
		} else {
			s.send(c, 8004, uid, 0, buildBossMonster8004Body(0, 0, 0, mailRewardItemCoins, uint32(reward.Coins)))
		}
	}
	if reward.Gold > 0 {
		if err := s.cfg.Store.AddGold(uid64, reward.Gold); err != nil {
			log.Printf("[mail] AddGold uid=%d: %v", uid, err)
		} else {
			s.send(c, 8004, uid, 0, buildBossMonster8004Body(0, 0, 0, mailRewardItemGold, uint32(reward.Gold)))
		}
	}
	for _, it := range reward.Items {
		if it.ItemID <= 0 || it.Count <= 0 {
			continue
		}
		if err := s.cfg.Store.AddItem(uid64, it.ItemID, it.Count); err != nil {
			log.Printf("[mail] AddItem uid=%d item=%d: %v", uid, it.ItemID, err)
			continue
		}
		s.send(c, 8004, uid, 0, buildBossMonster8004Body(0, 0, 0, uint32(it.ItemID), uint32(it.Count)))
	}
	petCount := reward.PetCount
	if petCount <= 0 {
		petCount = 1
	}
	if reward.PetID > 0 {
		name := "精灵"
		if s.cfg.Catalog != nil {
			if n := s.cfg.Catalog.PetNameOf(reward.PetID); n != "" {
				name = n
			}
		}
		for i := 0; i < petCount; i++ {
			catchTm := time.Now().Unix() + int64(i)
			np := &store.Pet{
				UserID:    uid64,
				CatchTime: catchTm,
				PetID:     reward.PetID,
				Name:      name,
				Level:     1,
				DV:        20,
				InBag:     true,
				BagPos:   99,
				Skills:    []int{10001},
			}
			n, _ := s.cfg.Store.CountBagPets(uid64)
			if n >= store.MaxBagPets {
				np.InBag = false
				np.BagPos = -1
			}
			if err := s.cfg.Store.UpsertPet(np); err != nil {
				log.Printf("[mail] UpsertPet uid=%d pet=%d: %v", uid, reward.PetID, err)
				continue
			}
			s.send(c, 8004, uid, 0, buildBossMonster8004Body(0, uint32(reward.PetID), uint32(np.CatchTime), 0, 0))
		}
	}
}

// handleMailDelete CMD 2755。
func (s *Server) handleMailDelete(c *Client, uid uint32, body []byte) {
	n := 0
	off := 0
	if len(body) >= 4 {
		n = int(binary.BigEndian.Uint32(body[0:4]))
		off = 4
	}
	ids := make([]int64, 0, n)
	for i := 0; i < n && off+4 <= len(body); i++ {
		ids = append(ids, int64(binary.BigEndian.Uint32(body[off:off+4])))
		off += 4
	}
	if s.cfg.Store != nil && len(ids) > 0 {
		_ = s.cfg.Store.DeleteMails(int64(uid), ids)
	}
	s.send(c, 2755, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d n=%d", cmdname.Format(2755), uid, len(ids))
}

// handleMailDelAll CMD 2756。
func (s *Server) handleMailDelAll(c *Client, uid uint32) {
	if s.cfg.Store != nil {
		_ = s.cfg.Store.DeleteAllMails(int64(uid))
	}
	s.send(c, 2756, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(2756), uid)
}

// handleMailUnread CMD 2757：unreadCount(4)。
func (s *Server) handleMailUnread(c *Client, uid uint32) {
	n := 0
	if s.cfg.Store != nil {
		if cnt, err := s.cfg.Store.CountUnreadMails(int64(uid)); err == nil {
			n = cnt
		}
	}
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, uint32(n))
	s.send(c, 2757, uid, 0, out)
}
