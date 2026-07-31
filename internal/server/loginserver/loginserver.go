package loginserver

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"

	"niaohao/server/internal/cmdname"
	"niaohao/server/internal/defaults"
	"niaohao/server/internal/packet"
	"niaohao/server/internal/store"
)

const (
	flashPolicyRequest = "<policy-file-request/>"
	maxPacketBytes     = 1 << 20
	maxAccumBytes      = 4 << 20
)

type Config struct {
	LoginPort     int
	GamePort      int
	ServerID      int
	AdvertiseHost string
	Store         *store.MySQL
	OnAuthed      func(userID int64)
}

type Server struct {
	cfg      Config
	listener net.Listener
	mu       sync.Mutex
	clients  map[net.Conn]struct{}
}

func New(cfg Config) *Server {
	if cfg.LoginPort == 0 {
		cfg.LoginPort = defaults.LoginTCP
	}
	if cfg.GamePort == 0 {
		cfg.GamePort = defaults.GameTCP
	}
	if cfg.ServerID == 0 {
		cfg.ServerID = 1
	}
	if strings.TrimSpace(cfg.AdvertiseHost) == "" {
		cfg.AdvertiseHost = "127.0.0.1"
	}
	return &Server{cfg: cfg, clients: make(map[net.Conn]struct{})}
}

func (s *Server) Start() error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", s.cfg.LoginPort))
	if err != nil {
		return err
	}
	s.listener = ln
	log.Printf("[login] TCP :%d (game advertise %s:%d)", s.cfg.LoginPort, s.cfg.AdvertiseHost, s.cfg.GamePort)
	go s.accept()
	return nil
}

func (s *Server) accept() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			log.Printf("[login] accept: %v", err)
			return
		}
		log.Printf("[login] conn + %s", conn.RemoteAddr())
		s.mu.Lock()
		s.clients[conn] = struct{}{}
		s.mu.Unlock()
		go s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	defer func() {
		s.mu.Lock()
		delete(s.clients, conn)
		s.mu.Unlock()
		_ = conn.Close()
	}()
	buf := make([]byte, 8192)
	var acc []byte
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		acc = append(acc, buf[:n]...)
		if len(acc) > maxAccumBytes {
			return
		}
		for s.consumePolicy(conn, &acc) {
		}
		s.process(&acc, conn)
	}
}

func (s *Server) consumePolicy(conn net.Conn, acc *[]byte) bool {
	b := *acc
	if len(b) < len(flashPolicyRequest) {
		return false
	}
	if string(b[:len(flashPolicyRequest)]) != flashPolicyRequest {
		return false
	}
	n := len(flashPolicyRequest)
	if len(b) > n && b[n] == 0 {
		n++
	}
	*acc = b[n:]
	policy := `<?xml version="1.0"?>
<!DOCTYPE cross-domain-policy SYSTEM "/xml/dtds/cross-domain-policy.dtd">
<cross-domain-policy>
	<allow-access-from domain="*" to-ports="*" />
</cross-domain-policy>` + "\x00"
	_, _ = conn.Write([]byte(policy))
	return true
}

func (s *Server) process(acc *[]byte, conn net.Conn) {
	b := *acc
	for len(b) >= packet.HeaderLen {
		pktLen := int(binary.BigEndian.Uint32(b[0:4]))
		if pktLen < packet.HeaderLen || pktLen > maxPacketBytes {
			*acc = nil
			return
		}
		if len(b) < pktLen {
			break
		}
		pkt := b[:pktLen]
		b = b[pktLen:]
		s.dispatch(conn, pkt)
	}
	*acc = b
}

func (s *Server) dispatch(conn net.Conn, pkt []byte) {
	_, cmd, uid, seq, body, err := packet.ParseHeader(pkt)
	if err != nil {
		return
	}
	userID := int64(uid)
	switch cmd {
	case 104:
		log.Printf("[CMD] OK     %s UID=%d SEQ=%d body=%d", cmdname.Format(cmd), userID, seq, len(body))
		s.handleEmailLogin(conn, userID, body)
	case 105:
		log.Printf("[CMD] OK     %s UID=%d SEQ=%d body=%d", cmdname.Format(cmd), userID, seq, len(body))
		s.handleCommendOnline(conn, userID)
	case 106:
		log.Printf("[CMD] OK     %s UID=%d SEQ=%d body=%d", cmdname.Format(cmd), userID, seq, len(body))
		s.handleRangeOnline(conn, userID, body)
	case 108:
		log.Printf("[CMD] OK     %s UID=%d SEQ=%d body=%d", cmdname.Format(cmd), userID, seq, len(body))
		s.handleCreateRole(conn, userID, body)
	case 1, 2, 1003: // SEER_VERIFY / REGISTER / REQUEST_REGISTER（本客户端主链走 104）
		log.Printf("[CMD] OK     %s UID=%d SEQ=%d body=%d (stub)", cmdname.Format(cmd), userID, seq, len(body))
		s.reply(conn, cmd, uid, 0, nil)
	case 7571, 80008:
		s.reply(conn, cmd, uid, 0, nil)
	default:
		log.Printf("[CMD] UNIMPL %s UID=%d SEQ=%d body=%d stage=登录服 -> 空回包",
			cmdname.Format(cmd), userID, seq, len(body))
		s.reply(conn, cmd, uid, 0, nil)
	}
}

func (s *Server) reply(conn net.Conn, cmd int32, uid uint32, result int32, body []byte) {
	_, _ = conn.Write(packet.BuildResponse(cmd, uid, result, body))
}

func (s *Server) handleEmailLogin(conn net.Conn, _ int64, body []byte) {
	if len(body) < 96 {
		s.reply(conn, 104, 0, 1, nil)
		return
	}
	email := packet.ReadFixedString(body[0:64])
	passMD5 := packet.ReadFixedString(body[64:96])
	log.Printf("[login-104] email=%s", email)

	user, err := s.cfg.Store.FindByEmail(email)
	if err != nil {
		log.Printf("[login-104] db: %v", err)
		s.reply(conn, 104, 0, 1, nil)
		return
	}
	var loginUID int64
	var errCode int32
	if user != nil {
		if passMD5 == user.Password {
			loginUID = user.UserID
		} else {
			errCode = 5003
		}
	} else {
		user, err = s.cfg.Store.CreateUser(email, passMD5)
		if err != nil || user == nil {
			log.Printf("[login-104] auto-reg fail: %v", err)
			s.reply(conn, 104, 0, 1, nil)
			return
		}
		loginUID = user.UserID
		log.Printf("[login-104] auto-reg uid=%d", loginUID)
	}

	session, sessionHex, err := store.NewSession()
	if err != nil {
		s.reply(conn, 104, 0, 1, nil)
		return
	}
	if user != nil && loginUID > 0 && errCode == 0 {
		user.SessionHex = sessionHex
		user.LastOnline = 0
		_ = s.cfg.Store.SaveUser(user)
	}

	roleCreate := uint32(0)
	if user != nil && user.RoleCreated {
		roleCreate = 1
	}
	resp := make([]byte, 20)
	copy(resp[0:16], session)
	binary.BigEndian.PutUint32(resp[16:20], roleCreate)
	s.reply(conn, 104, uint32(loginUID), errCode, resp)

	if errCode == 0 && loginUID > 0 {
		log.Printf("[login-104] ok uid=%d roleCreated=%d", loginUID, roleCreate)
		if s.cfg.OnAuthed != nil {
			s.cfg.OnAuthed(loginUID)
		}
	}
}

func (s *Server) handleCreateRole(conn net.Conn, userID int64, body []byte) {
	if len(body) < 24 {
		s.reply(conn, 108, uint32(userID), 1, nil)
		return
	}
	nick := packet.ReadFixedString(body[4:20])
	color := int(binary.BigEndian.Uint32(body[20:24]))
	if nick == "" {
		nick = fmt.Sprintf("%d", userID)
	}
	user, err := s.cfg.Store.FindByUserID(userID)
	if err != nil || user == nil {
		s.reply(conn, 108, uint32(userID), 1, nil)
		return
	}
	user.RoleCreated = true
	user.Nickname = nick
	user.Color = color
	user.MapID = 1
	user.PosX = 475
	user.PosY = 395

	// 本客户端创角后仍携带 104 的 session 进 1001；此处不得轮换 session，否则必 mismatch。
	session := make([]byte, 16)
	if user.SessionHex != "" {
		if b, err := hex.DecodeString(user.SessionHex); err == nil && len(b) == 16 {
			copy(session, b)
		}
	}
	if isZeroSession(session) {
		raw, sessionHex, err := store.NewSession()
		if err != nil {
			s.reply(conn, 108, uint32(userID), 1, nil)
			return
		}
		copy(session, raw)
		user.SessionHex = sessionHex
	}

	if err := s.cfg.Store.SaveUser(user); err != nil {
		s.reply(conn, 108, uint32(userID), 1, nil)
		return
	}
	s.reply(conn, 108, uint32(userID), 0, session)
	log.Printf("[login-108] create role uid=%d nick=%s color=%d session=%s", userID, nick, color, user.SessionHex)
}

func isZeroSession(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}

func (s *Server) handleCommendOnline(conn net.Conn, userID int64) {
	body := s.build105()
	s.reply(conn, 105, uint32(userID), 0, body)
	log.Printf("[login-105] game %s:%d", s.cfg.AdvertiseHost, s.cfg.GamePort)
}

func (s *Server) handleRangeOnline(conn net.Conn, userID int64, body []byte) {
	servers := []serverInfo{s.oneServer()}
	startID, endID := 1, 1
	if len(body) >= 8 {
		startID = int(binary.BigEndian.Uint32(body[0:4]))
		endID = int(binary.BigEndian.Uint32(body[4:8]))
	}
	subset := make([]serverInfo, 0)
	for i := startID; i <= endID && i-1 < len(servers); i++ {
		subset = append(subset, servers[i-1])
	}
	buf := make([]byte, 4+len(subset)*30)
	binary.BigEndian.PutUint32(buf[0:4], uint32(len(subset)))
	off := 4
	for _, sv := range subset {
		off = writeServerInfo(buf, off, sv)
	}
	s.reply(conn, 106, uint32(userID), 0, buf)
}

type serverInfo struct {
	ID, UserCount, Port, Friends int
	IP                           string
}

func (s *Server) oneServer() serverInfo {
	return serverInfo{
		ID: s.cfg.ServerID, UserCount: 0, IP: s.cfg.AdvertiseHost,
		Port: s.cfg.GamePort, Friends: 1,
	}
}

func (s *Server) build105() []byte {
	sv := s.oneServer()
	// maxOnlineID + isVIP + onlineCnt + ServerInfo*n + friendCnt + blackCnt
	n := 1
	total := 12 + n*30 + 4 + 4
	buf := make([]byte, total)
	off := 0
	binary.BigEndian.PutUint32(buf[off:], 18)
	off += 4
	binary.BigEndian.PutUint32(buf[off:], 0)
	off += 4
	binary.BigEndian.PutUint32(buf[off:], uint32(n))
	off += 4
	off = writeServerInfo(buf, off, sv)
	binary.BigEndian.PutUint32(buf[off:], 0) // friends
	off += 4
	binary.BigEndian.PutUint32(buf[off:], 0) // blacks
	return buf
}

func writeServerInfo(buf []byte, off int, s serverInfo) int {
	binary.BigEndian.PutUint32(buf[off:], uint32(s.ID))
	off += 4
	binary.BigEndian.PutUint32(buf[off:], uint32(s.UserCount))
	off += 4
	ipb := []byte(s.IP)
	if len(ipb) > 16 {
		ipb = ipb[:16]
	}
	copy(buf[off:off+16], ipb)
	off += 16
	binary.BigEndian.PutUint16(buf[off:], uint16(s.Port))
	off += 2
	binary.BigEndian.PutUint32(buf[off:], uint32(s.Friends))
	off += 4
	return off
}
