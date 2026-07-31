package store

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// JSON 存档布局：
//   <dir>/meta.json          nextID + email→uid
//   <dir>/users/<uid>.json   单用户全量文档
//   <dir>/gm_audit.jsonl     审计追加

type jsonStore struct {
	dir    string
	mu     sync.Mutex
	nextID atomic.Int64
	emails map[string]int64 // email → uid
	mailSeq atomic.Int64
	auditSeq atomic.Int64
}

type jsonMeta struct {
	NextID   int64            `json:"nextId"`
	Emails   map[string]int64 `json:"emails"`
	MailSeq  int64            `json:"mailSeq"`
	AuditSeq int64            `json:"auditSeq"`
}

type jsonUserDoc struct {
	User           User                   `json:"user"`
	ExpPool        int                    `json:"expPool"`
	Pets           []Pet                  `json:"pets"`
	Items          map[string]Item        `json:"items"` // key=itemID
	Tasks          map[string]Task        `json:"tasks"` // key=taskID
	Mails          []Mail                 `json:"mails"`
	Friends        []FriendEntry          `json:"friends"`
	Blacklist      []BlackEntry           `json:"blacklist"`
	Clothes        []WornCloth            `json:"clothes"`
	Progress       UserProgress           `json:"progress"`
	GaiyaDef       int                    `json:"gaiyaDef"`
	GaiyaMask      int                    `json:"gaiyaMask"`
	RecruitMask    uint32                 `json:"recruitMask"`
	PetCollectMask int                    `json:"petCollectMask"`
	PetKing301     bool                   `json:"petKing301"`
	SoulBeads      []SoulBead             `json:"soulBeads"`
	Hatch          HatchState             `json:"hatch"`
	Nono           *Nono                  `json:"nono,omitempty"`
	Equips         map[string]int         `json:"equips"` // itemID → enhance_lv
	AchieveRules   []AchieveRuleRow       `json:"achieveRules"`
	AchieveBranch  map[string][2]int      `json:"achieveBranch"` // "branchID" → [value,status]
	Titles         []int                  `json:"titles"`
	SPTDefeated    []int                  `json:"sptDefeated"`
	Boost          BoostTimes             `json:"boost"`
	GoldPromoN     int                    `json:"goldPromoN,omitempty"`
	Fitments       []Fitment              `json:"fitments,omitempty"`
	Breed          BreedState             `json:"breed,omitempty"`
	RoomPets       RoomPets               `json:"roomPets,omitempty"`
	NonoVipSign    NonoVipSignState       `json:"nonoVipSign,omitempty"`
	LeiyiTrain     LeiyiTrainProgress     `json:"leiyiTrain,omitempty"`
	UserOps        UserOpsState           `json:"userOps,omitempty"`
}

type jsonAuditRow struct {
	ID         int64           `json:"id"`
	Admin      string          `json:"admin"`
	Action     string          `json:"action"`
	TargetUser int64           `json:"targetUser"`
	Detail     json.RawMessage `json:"detail,omitempty"`
	CreatedAt  int64           `json:"createdAt"`
}

func openJSONStore(dir string) (*jsonStore, error) {
	if err := os.MkdirAll(filepath.Join(dir, "users"), 0o755); err != nil {
		return nil, err
	}
	s := &jsonStore{
		dir:    dir,
		emails: make(map[string]int64),
	}
	metaPath := filepath.Join(dir, "meta.json")
	if b, err := os.ReadFile(metaPath); err == nil {
		var m jsonMeta
		if json.Unmarshal(b, &m) == nil {
			if m.Emails != nil {
				s.emails = m.Emails
			}
			if m.NextID >= MinUserID {
				s.nextID.Store(m.NextID)
			} else {
				s.nextID.Store(MinUserID)
			}
			s.mailSeq.Store(m.MailSeq)
			s.auditSeq.Store(m.AuditSeq)
		}
	} else {
		s.nextID.Store(MinUserID)
	}
	// 扫描已有用户补全 email 索引
	entries, _ := os.ReadDir(filepath.Join(dir, "users"))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		uid, err := strconv.ParseInt(strings.TrimSuffix(e.Name(), ".json"), 10, 64)
		if err != nil {
			continue
		}
		doc, err := s.loadDoc(uid)
		if err != nil || doc == nil {
			continue
		}
		email := strings.TrimSpace(strings.ToLower(doc.User.Email))
		if email != "" {
			s.emails[email] = uid
		}
		if uid > s.nextID.Load() {
			s.nextID.Store(uid)
		}
	}
	if err := s.migrateLowUserIDs(); err != nil {
		log.Printf("[store] JSON UID migrate warn: %v", err)
	}
	if cur := s.nextID.Load(); cur < MinUserID {
		s.nextID.Store(MinUserID)
	}
	_ = s.saveMeta()
	return s, nil
}

func (s *jsonStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveMetaLocked()
}

func (s *jsonStore) Ping() error {
	_, err := os.Stat(s.dir)
	return err
}

func (s *jsonStore) saveMeta() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveMetaLocked()
}

func (s *jsonStore) saveMetaLocked() error {
	m := jsonMeta{
		NextID:   s.nextID.Load(),
		Emails:   s.emails,
		MailSeq:  s.mailSeq.Load(),
		AuditSeq: s.auditSeq.Load(),
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir, "meta.json"), b, 0o644)
}

func (s *jsonStore) userPath(uid int64) string {
	return filepath.Join(s.dir, "users", strconv.FormatInt(uid, 10)+".json")
}

func (s *jsonStore) loadDoc(uid int64) (*jsonUserDoc, error) {
	b, err := os.ReadFile(s.userPath(uid))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	doc := &jsonUserDoc{}
	if err := json.Unmarshal(b, doc); err != nil {
		return nil, err
	}
	if doc.Items == nil {
		doc.Items = map[string]Item{}
	}
	if doc.Tasks == nil {
		doc.Tasks = map[string]Task{}
	}
	if doc.Equips == nil {
		doc.Equips = map[string]int{}
	}
	if doc.AchieveBranch == nil {
		doc.AchieveBranch = map[string][2]int{}
	}
	if doc.Progress.BraveCur < 1 {
		doc.Progress = UserProgress{BraveCur: 1, BraveMax: 1, FreshCur: 1, FreshMax: 1}
	}
	return doc, nil
}

func (s *jsonStore) saveDoc(doc *jsonUserDoc) error {
	if doc == nil {
		return fmt.Errorf("nil doc")
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.userPath(doc.User.UserID), b, 0o644)
}

func (s *jsonStore) withDoc(uid int64, fn func(*jsonUserDoc) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil {
		return err
	}
	if doc == nil {
		return fmt.Errorf("user %d not found", uid)
	}
	if err := fn(doc); err != nil {
		return err
	}
	return s.saveDoc(doc)
}

func (s *jsonStore) emptyDoc(u User) *jsonUserDoc {
	return &jsonUserDoc{
		User:          u,
		Items:         map[string]Item{},
		Tasks:         map[string]Task{},
		Equips:        map[string]int{},
		AchieveBranch: map[string][2]int{},
		Progress:      UserProgress{BraveCur: 1, BraveMax: 1, FreshCur: 1, FreshMax: 1},
		Pets:          []Pet{},
		Mails:         []Mail{},
		Friends:       []FriendEntry{},
		Blacklist:     []BlackEntry{},
		Clothes:       []WornCloth{},
		Titles:        []int{},
		SPTDefeated:   []int{},
		AchieveRules:  []AchieveRuleRow{},
	}
}
