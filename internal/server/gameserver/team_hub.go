package gameserver

import (
	"encoding/binary"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	teamMinID            = 50001
	simpleTeamInfoLen    = 184
	teamLogoInfoLen      = 16
	teamInformBodyLen    = 32
	teamMemberEntryLen   = 12
	teamPrivLeader       = 0
	teamPrivVice         = 1
	teamPrivMember       = 5
	teamDefaultLogoWord  = "TEAM"
	teamDefaultSlogan    = "欢迎加入战队"
)

// teamMember 战队成员运行时。
type teamMember struct {
	UserID         int64  `json:"userId"`
	Priv           uint32 `json:"priv"`
	Contribute     uint32 `json:"contribute"`
	AllContrib     uint32 `json:"allContrib"`
	CanExContrib   uint32 `json:"canExContrib"`
	IsShow         bool   `json:"isShow"`
}

// teamArmPlace 要塞装饰摆放（2941/2944，20B/条）。
type teamArmPlace struct {
	ID     uint32 `json:"id"`
	X      uint32 `json:"x"`
	Y      uint32 `json:"y"`
	Dir    uint32 `json:"dir"`
	Status uint32 `json:"status"`
}

// teamArmStock 装饰仓库（2942，12B/条：id+used+all）。
type teamArmStock struct {
	ID   uint32 `json:"id"`
	Used uint32 `json:"used"`
	All  uint32 `json:"all"`
}

// teamArmUp 升级建筑（2965/2966/2967）。
type teamArmUp struct {
	ID       uint32    `json:"id"`
	BuyTime  uint32    `json:"buyTime"`
	Form     uint32    `json:"form"`
	HP       uint32    `json:"hp"`
	Work     uint32    `json:"work"`
	Donate   uint32    `json:"donate"`
	Res      [4]uint32 `json:"res"`
	X        uint32    `json:"x"`
	Y        uint32    `json:"y"`
	Dir      uint32    `json:"dir"`
	Status   uint32    `json:"status"`
	IsUsed   bool      `json:"isUsed"`
}

// teamRuntime 战队完整数据（落盘 teams.json）。
type teamRuntime struct {
	ID             uint32                `json:"id"`
	LeaderID       int64                 `json:"leaderId"`
	Name           string                `json:"name"`
	Slogan         string                `json:"slogan"`
	Notice         string                `json:"notice"`
	Interest       uint32                `json:"interest"`
	JoinFlag       uint32                `json:"joinFlag"` // 0 自由 / 1 审批 / 其它禁止
	VisitFlag      uint32                `json:"visitFlag"`
	Exp            uint32                `json:"exp"`
	Score          uint32                `json:"score"`
	SuperCoreNum   uint32                `json:"superCoreNum"`
	LogoBg         uint16                `json:"logoBg"`
	LogoIcon       uint16                `json:"logoIcon"`
	LogoColor      uint16                `json:"logoColor"`
	TxtColor       uint16                `json:"txtColor"`
	LogoWord       string                `json:"logoWord"`
	HeadquartersID uint32                `json:"headquartersId"`
	Arms           []teamArmPlace        `json:"arms"`
	ArmStock       []teamArmStock        `json:"armStock"`
	UpArms         []teamArmUp           `json:"upArms"`
	Members        map[int64]*teamMember `json:"-"`
	MemberList     []teamMember          `json:"members"` // 仅序列化
}

type teamPersistFile struct {
	NextID  uint32         `json:"nextId"`
	Teams   []*teamRuntime `json:"teams"`
	Pending map[string]uint32 `json:"pending"` // uidStr → teamID
}

type teamHub struct {
	mu            sync.Mutex
	path          string
	nextID        uint32
	teams         map[uint32]*teamRuntime
	uidIndex      map[int64]uint32 // uid → teamID
	pendingByUser map[int64]uint32
}

func (s *Server) initTeamHub() {
	if s.teams != nil {
		return
	}
	path := ""
	if s.cfg.DataDir != "" {
		path = filepath.Join(s.cfg.DataDir, "saves", "teams.json")
	}
	h := &teamHub{
		path:          path,
		nextID:        teamMinID,
		teams:         make(map[uint32]*teamRuntime),
		uidIndex:      make(map[int64]uint32),
		pendingByUser: make(map[int64]uint32),
	}
	h.load()
	s.teams = h
	log.Printf("[team] hub ready path=%s teams=%d nextID=%d", path, len(h.teams), h.nextID)
}

func (h *teamHub) load() {
	if h.path == "" {
		return
	}
	b, err := os.ReadFile(h.path)
	if err != nil || len(b) == 0 {
		return
	}
	var f teamPersistFile
	if json.Unmarshal(b, &f) != nil {
		log.Printf("[team] load corrupt %s", h.path)
		return
	}
	if f.NextID >= teamMinID {
		h.nextID = f.NextID
	}
	for _, t := range f.Teams {
		if t == nil || t.ID < teamMinID {
			continue
		}
		t.Members = make(map[int64]*teamMember)
		for i := range t.MemberList {
			m := t.MemberList[i]
			cp := m
			t.Members[m.UserID] = &cp
			h.uidIndex[m.UserID] = t.ID
		}
		h.teams[t.ID] = t
		if t.ID >= h.nextID {
			h.nextID = t.ID + 1
		}
	}
	for k, tid := range f.Pending {
		var uid int64
		if _, err := fmtSscanf(k, &uid); err == nil && uid > 0 {
			h.pendingByUser[uid] = tid
		}
	}
}

func fmtSscanf(s string, uid *int64) (int, error) {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, os.ErrInvalid
		}
		n = n*10 + int64(c-'0')
	}
	*uid = n
	return 1, nil
}

func (h *teamHub) saveLocked() {
	if h.path == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(h.path), 0o755)
	f := teamPersistFile{
		NextID:  h.nextID,
		Teams:   make([]*teamRuntime, 0, len(h.teams)),
		Pending: make(map[string]uint32, len(h.pendingByUser)),
	}
	for _, t := range h.teams {
		t.MemberList = t.MemberList[:0]
		for _, m := range t.Members {
			if m != nil {
				t.MemberList = append(t.MemberList, *m)
			}
		}
		f.Teams = append(f.Teams, t)
	}
	for uid, tid := range h.pendingByUser {
		f.Pending[itoa64(uid)] = tid
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return
	}
	tmp := h.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, h.path)
}

func itoa64(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [32]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func (h *teamHub) getByID(id uint32) *teamRuntime {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.teams[id]
}

func (h *teamHub) teamIDOf(uid int64) uint32 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.uidIndex[uid]
}

func (h *teamHub) memberOf(uid int64) (teamID uint32, m *teamMember, t *teamRuntime) {
	h.mu.Lock()
	defer h.mu.Unlock()
	teamID = h.uidIndex[uid]
	if teamID == 0 {
		return 0, nil, nil
	}
	t = h.teams[teamID]
	if t == nil {
		return 0, nil, nil
	}
	return teamID, t.Members[uid], t
}

func teamPutFixed(dst []byte, off int, s string, n int) {
	if off+n > len(dst) {
		return
	}
	for i := 0; i < n; i++ {
		dst[off+i] = 0
	}
	b := []byte(s)
	if len(b) > n {
		// 按 rune 截断到不超过 n 字节
		b = truncateUTF8(b, n)
	}
	copy(dst[off:], b)
}

func truncateUTF8(b []byte, n int) []byte {
	if len(b) <= n {
		return b
	}
	for n > 0 && !utf8.RuneStart(b[n]) {
		n--
	}
	return b[:n]
}

func teamBuildSimpleInfoBody(t *teamRuntime) []byte {
	body := make([]byte, simpleTeamInfoLen)
	if t == nil {
		return body
	}
	binary.BigEndian.PutUint32(body[0:4], t.ID)
	binary.BigEndian.PutUint32(body[4:8], uint32(t.LeaderID))
	binary.BigEndian.PutUint32(body[8:12], t.SuperCoreNum)
	mc := uint32(0)
	if t.Members != nil {
		mc = uint32(len(t.Members))
	}
	binary.BigEndian.PutUint32(body[12:16], mc)
	binary.BigEndian.PutUint32(body[16:20], t.Interest)
	binary.BigEndian.PutUint32(body[20:24], t.JoinFlag)
	binary.BigEndian.PutUint32(body[24:28], t.VisitFlag)
	binary.BigEndian.PutUint32(body[28:32], t.Exp)
	binary.BigEndian.PutUint32(body[32:36], t.Score)
	teamPutFixed(body, 36, t.Name, 16)
	teamPutFixed(body, 52, t.Slogan, 60)
	teamPutFixed(body, 112, t.Notice, 60)
	binary.BigEndian.PutUint16(body[172:174], t.LogoBg)
	binary.BigEndian.PutUint16(body[174:176], t.LogoIcon)
	binary.BigEndian.PutUint16(body[176:178], t.LogoColor)
	binary.BigEndian.PutUint16(body[178:180], t.TxtColor)
	teamPutFixed(body, 180, t.LogoWord, 4)
	return body
}

func teamBuildLogoInfoBody(id uint32, t *teamRuntime) []byte {
	body := make([]byte, teamLogoInfoLen)
	binary.BigEndian.PutUint32(body[0:4], id)
	if t != nil {
		binary.BigEndian.PutUint16(body[4:6], t.LogoBg)
		binary.BigEndian.PutUint16(body[6:8], t.LogoIcon)
		binary.BigEndian.PutUint16(body[8:10], t.LogoColor)
		binary.BigEndian.PutUint16(body[10:12], t.TxtColor)
		teamPutFixed(body, 12, t.LogoWord, 4)
	}
	return body
}

func teamBuildInformBody(msgType, actorUID uint32, nick string, data1, data2 uint32) []byte {
	body := make([]byte, teamInformBodyLen)
	binary.BigEndian.PutUint32(body[0:4], msgType)
	binary.BigEndian.PutUint32(body[4:8], actorUID)
	teamPutFixed(body, 8, nick, 16)
	binary.BigEndian.PutUint32(body[24:28], data1)
	binary.BigEndian.PutUint32(body[28:32], data2)
	return body
}

// userTeamSnapshot 登录/地图用的个人战队摘要。
type userTeamSnapshot struct {
	ID           uint32
	Priv         uint32
	SuperCore    uint32
	IsShow       uint32
	AllContrib   uint32
	CanExContrib uint32
	LogoBg       uint16
	LogoIcon     uint16
	LogoColor    uint16
	TxtColor     uint16
	LogoWord     string
	CoreCount    uint32
}

func (s *Server) userTeamSnapshot(uid int64) userTeamSnapshot {
	s.initTeamHub()
	h := s.teams
	h.mu.Lock()
	defer h.mu.Unlock()
	tid := h.uidIndex[uid]
	if tid == 0 {
		return userTeamSnapshot{Priv: teamPrivMember}
	}
	t := h.teams[tid]
	if t == nil {
		return userTeamSnapshot{Priv: teamPrivMember}
	}
	m := t.Members[uid]
	snap := userTeamSnapshot{
		ID:        tid,
		Priv:      teamPrivMember,
		IsShow:    1,
		LogoBg:    t.LogoBg,
		LogoIcon:  t.LogoIcon,
		LogoColor: t.LogoColor,
		TxtColor:  t.TxtColor,
		LogoWord:  t.LogoWord,
		CoreCount: t.SuperCoreNum,
	}
	if m != nil {
		snap.Priv = m.Priv
		snap.AllContrib = m.AllContrib
		snap.CanExContrib = m.CanExContrib
		if m.IsShow {
			snap.IsShow = 1
		} else {
			snap.IsShow = 0
		}
		if m.Priv == 0 && t.SuperCoreNum > 0 {
			snap.SuperCore = 1
		}
	}
	return snap
}

func (s *Server) teamNick(uid int64) string {
	if s.cfg.Store != nil {
		if u, err := s.cfg.Store.FindByUserID(uid); err == nil && u != nil && strings.TrimSpace(u.Nickname) != "" {
			return u.Nickname
		}
	}
	return itoa64(uid)
}

func (s *Server) pushTeamInform(targetUID int64, msgType, actorUID uint32, nick string, data1, data2 uint32) {
	body := teamBuildInformBody(msgType, actorUID, nick, data1, data2)
	if c := s.clientOf(targetUID); c != nil && c.LoggedIn {
		s.send(c, 2913, uint32(targetUID), 0, body)
	}
}

// parseTeamCreateBody 对齐 TeamCreater SWF：writeUTFBytes 定长 + writeShort 队徽。
// 布局：name16 + slogan60 + interest4 + joinFlag4 + logoBg/Icon/Color/txtColor(各2) + logoWord4 = 96B。
// 兼容：全 u32 队徽（5×u32）接在 name/slogan 后。
func parseTeamCreateBody(body []byte) (name, slogan string, interest, joinFlag uint32, bg, icon, color, txt uint16, word string) {
	name = "战队"
	slogan = teamDefaultSlogan
	word = teamDefaultLogoWord
	joinFlag = 0
	if len(body) >= 16 {
		name = strings.Trim(string(body[0:16]), "\x00")
	}
	if len(body) >= 76 {
		slogan = strings.Trim(string(body[16:76]), "\x00")
	}
	if len(body) >= 80 {
		interest = binary.BigEndian.Uint32(body[76:80])
	}
	if len(body) >= 84 {
		joinFlag = binary.BigEndian.Uint32(body[80:84])
	}
	if len(body) >= 96 {
		// shorts
		bg = binary.BigEndian.Uint16(body[84:86])
		icon = binary.BigEndian.Uint16(body[86:88])
		color = binary.BigEndian.Uint16(body[88:90])
		txt = binary.BigEndian.Uint16(body[90:92])
		word = strings.Trim(string(body[92:96]), "\x00")
	} else if len(body) >= 84+20 {
		bg = uint16(binary.BigEndian.Uint32(body[84:88]))
		icon = uint16(binary.BigEndian.Uint32(body[88:92]))
		color = uint16(binary.BigEndian.Uint32(body[92:96]))
		txt = uint16(binary.BigEndian.Uint32(body[96:100]))
		if len(body) >= 104 {
			buf := make([]byte, 4)
			binary.BigEndian.PutUint32(buf, binary.BigEndian.Uint32(body[100:104]))
			word = strings.Trim(string(buf), "\x00")
		}
	}
	if name == "" {
		name = "战队"
	}
	if slogan == "" {
		slogan = teamDefaultSlogan
	}
	if word == "" {
		word = teamDefaultLogoWord
	}
	return
}

func applyTeamModifyLogoBody(body []byte, t *teamRuntime) {
	if t == nil {
		return
	}
	if len(body) >= 16 {
		t.LogoBg = uint16(binary.BigEndian.Uint32(body[0:4]))
		t.LogoIcon = uint16(binary.BigEndian.Uint32(body[4:8]))
		t.LogoColor = uint16(binary.BigEndian.Uint32(body[8:12]))
		t.TxtColor = uint16(binary.BigEndian.Uint32(body[12:16]))
		if len(body) >= 20 {
			buf := make([]byte, 4)
			binary.BigEndian.PutUint32(buf, binary.BigEndian.Uint32(body[16:20]))
			t.LogoWord = strings.Trim(string(buf), "\x00")
		} else if len(body) > 16 {
			t.LogoWord = strings.Trim(string(body[16:]), "\x00")
			if len(t.LogoWord) > 4 {
				t.LogoWord = t.LogoWord[:4]
			}
		}
		return
	}
	if len(body) >= 8 {
		t.LogoBg = binary.BigEndian.Uint16(body[0:2])
		t.LogoIcon = binary.BigEndian.Uint16(body[2:4])
		t.LogoColor = binary.BigEndian.Uint16(body[4:6])
		t.TxtColor = binary.BigEndian.Uint16(body[6:8])
	}
	if len(body) >= 12 {
		t.LogoWord = strings.Trim(string(body[8:12]), "\x00")
	}
}

func teamParseChatBody(body []byte, defaultTeamID uint32) (teamID uint32, msg string) {
	teamID = defaultTeamID
	if len(body) < 8 {
		return teamID, strings.Trim(string(body), "\x00")
	}
	// teamID(4)+msgLen(4)+msg
	cand := binary.BigEndian.Uint32(body[0:4])
	msgLen := binary.BigEndian.Uint32(body[4:8])
	if cand >= teamMinID && int(8+msgLen) <= len(body) {
		return cand, string(body[8 : 8+msgLen])
	}
	return teamID, strings.Trim(string(body), "\x00")
}
