package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type User struct {
	UserID       int64
	Email        string
	Password     string // 存客户端传来的 32 位 MD5 hex
	Nickname     string
	Color        int
	RoleCreated  bool
	Coins        int
	Gold         int
	Energy       int
	ExpTotal     int64
	MapID        int
	PosX         int
	PosY         int
	SessionHex   string
	LastOnline   int64
	LoginCnt     int
	RegisterTime int64
}

// sqlBackend MySQL 实现（内部）。
type sqlBackend struct {
	db     *sql.DB
	nextID atomic.Int64
	mu     sync.Mutex
}

// MySQL 对外存档句柄：优先 MySQL，不可用时走 JSON 文件。
type MySQL struct {
	sql  *sqlBackend
	json *jsonStore
	mode string // "mysql" | "json"
}

type Config struct {
	DSN string
}

// Backend 返回当前后端：mysql 或 json。
func (s *MySQL) Backend() string {
	if s == nil {
		return ""
	}
	return s.mode
}

func OpenMySQL(dsn string) (*MySQL, error) {
	// 若库不存在则先建库（DSN 需指向目标库）
	if err := ensureDatabase(dsn); err != nil {
		return nil, err
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	be := &sqlBackend{db: db}
	if err := be.ensureSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := be.migrateLowUserIDs(); err != nil {
		log.Printf("[store] UID migrate warn: %v", err)
	}
	var maxID sql.NullInt64
	_ = db.QueryRow(`SELECT MAX(user_id) FROM users`).Scan(&maxID)
	if maxID.Valid && maxID.Int64 >= MinUserID {
		be.nextID.Store(maxID.Int64)
	} else {
		be.nextID.Store(MinUserID)
	}
	return &MySQL{sql: be, mode: "mysql"}, nil
}

// OpenJSON 纯 JSON 文件存档（无 MySQL 时使用）。
func OpenJSON(dir string) (*MySQL, error) {
	js, err := openJSONStore(dir)
	if err != nil {
		return nil, err
	}
	return &MySQL{json: js, mode: "json"}, nil
}

// OpenAuto 优先连接 MySQL；失败则回落 JSON 目录（默认 data/saves）。
func OpenAuto(dsn, jsonDir string) (db *MySQL, backend string, err error) {
	db, err = OpenMySQL(dsn)
	if err == nil {
		return db, "mysql", nil
	}
	mysqlErr := err
	if jsonDir == "" {
		jsonDir = "data/saves"
	}
	db, err = OpenJSON(jsonDir)
	if err != nil {
		return nil, "", fmt.Errorf("mysql: %v; json: %w", mysqlErr, err)
	}
	return db, "json", nil
}

func ensureDatabase(dsn string) error {
	// user:pass@tcp(host:port)/dbname?params → 连到无库名并 CREATE DATABASE
	schemeEnd := strings.Index(dsn, "@tcp(")
	if schemeEnd < 0 {
		return nil
	}
	pathStart := strings.Index(dsn[schemeEnd:], ")/")
	if pathStart < 0 {
		return nil
	}
	pathStart = schemeEnd + pathStart + 2 // after )/
	rest := dsn[pathStart:]
	dbName := rest
	params := ""
	if i := strings.Index(rest, "?"); i >= 0 {
		dbName = rest[:i]
		params = rest[i:]
	}
	if dbName == "" {
		return nil
	}
	baseDSN := dsn[:pathStart] + params
	db, err := sql.Open("mysql", baseDSN)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec("CREATE DATABASE IF NOT EXISTS `" + dbName + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci")
	return err
}

func (s *sqlBackend) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *sqlBackend) Ping() error {
	return s.db.Ping()
}

func (s *sqlBackend) DB() *sql.DB { return s.db }

func (s *sqlBackend) ensureSchema() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
  user_id BIGINT PRIMARY KEY,
  email VARCHAR(128) NOT NULL UNIQUE,
  password VARCHAR(128) NOT NULL,
  nickname VARCHAR(64) NOT NULL DEFAULT '',
  color INT NOT NULL DEFAULT 0,
  role_created TINYINT NOT NULL DEFAULT 0,
  coins INT NOT NULL DEFAULT 0,
  gold INT NOT NULL DEFAULT 0,
  energy INT NOT NULL DEFAULT 100,
  exp_total BIGINT NOT NULL DEFAULT 0,
  exp_pool INT NOT NULL DEFAULT 0,
  map_id INT NOT NULL DEFAULT 1,
  pos_x INT NOT NULL DEFAULT 300,
  pos_y INT NOT NULL DEFAULT 270,
  current_server INT NOT NULL DEFAULT 0,
  session_hex VARCHAR(64) NOT NULL DEFAULT '',
  last_online BIGINT NOT NULL DEFAULT 0,
  login_cnt INT NOT NULL DEFAULT 0,
  login_streak INT NOT NULL DEFAULT 0,
  register_time BIGINT NOT NULL DEFAULT 0,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX idx_nickname (nickname)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS pets (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT NOT NULL,
  catch_time BIGINT NOT NULL,
  pet_id INT NOT NULL,
  pet_name VARCHAR(64) NOT NULL DEFAULT '',
  level INT NOT NULL DEFAULT 1,
  exp INT NOT NULL DEFAULT 0,
  dv INT NOT NULL DEFAULT 0,
  nature INT NOT NULL DEFAULT 0,
  bag_pos INT NOT NULL DEFAULT -1,
  in_bag TINYINT NOT NULL DEFAULT 1,
  skills_json JSON NOT NULL,
  extra_json JSON NULL,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_user_catch (user_id, catch_time),
  INDEX idx_user_pet (user_id, pet_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS user_tasks (
  user_id BIGINT NOT NULL,
  task_id INT NOT NULL,
  status TINYINT NOT NULL DEFAULT 0,
  buf_blob VARBINARY(20) NOT NULL DEFAULT '',
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, task_id),
  INDEX idx_user_status (user_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS items (
  user_id BIGINT NOT NULL,
  item_id INT NOT NULL,
  count INT NOT NULL DEFAULT 0,
  expire_time INT NOT NULL DEFAULT 0,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, item_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS props (
  user_id BIGINT NOT NULL,
  item_id INT NOT NULL,
  count INT NOT NULL DEFAULT 0,
  expire_time INT NOT NULL DEFAULT 0,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, item_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS exp_ledger (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT NOT NULL,
  pet_row_id BIGINT NOT NULL DEFAULT 0,
  delta INT NOT NULL,
  level_after INT NOT NULL DEFAULT 0,
  exp_after INT NOT NULL DEFAULT 0,
  reason VARCHAR(64) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_user_time (user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS gold_ledger (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT NOT NULL,
  delta INT NOT NULL,
  balance_after INT NOT NULL,
  reason VARCHAR(64) NOT NULL DEFAULT '',
  ref_id VARCHAR(64) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_user_time (user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS equips (
  user_id BIGINT NOT NULL,
  item_id INT NOT NULL,
  count INT NOT NULL DEFAULT 1,
  enhance_lv INT NOT NULL DEFAULT 0,
  equipped_slot INT NOT NULL DEFAULT 0,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, item_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS soul_beads (
  user_id BIGINT NOT NULL,
  item_id INT NOT NULL,
  count INT NOT NULL DEFAULT 0,
  hatch_progress INT NOT NULL DEFAULT 0,
  extra_json JSON NULL,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, item_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS essences (
  user_id BIGINT NOT NULL,
  item_id INT NOT NULL,
  count INT NOT NULL DEFAULT 0,
  breed_mon_id INT NOT NULL DEFAULT 0,
  breed_time INT NOT NULL DEFAULT 0,
  hatch_start BIGINT NOT NULL DEFAULT 0,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, item_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS gm_audit (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  admin VARCHAR(64) NOT NULL,
  action VARCHAR(64) NOT NULL,
  target_user BIGINT NOT NULL DEFAULT 0,
  detail_json JSON NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_time (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS user_friends (
  user_id BIGINT NOT NULL,
  friend_id BIGINT NOT NULL,
  time_poke INT NOT NULL DEFAULT 0,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, friend_id),
  INDEX idx_friend (friend_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS user_blacklist (
  user_id BIGINT NOT NULL,
  black_id BIGINT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, black_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS user_worn_clothes (
  user_id BIGINT NOT NULL,
  slot_idx INT NOT NULL,
  item_id INT NOT NULL,
  level INT NOT NULL DEFAULT 0,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, slot_idx)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS user_mails (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT NOT NULL,
  template INT NOT NULL DEFAULT 0,
  mail_time BIGINT NOT NULL DEFAULT 0,
  from_id BIGINT NOT NULL DEFAULT 0,
  from_nick VARCHAR(16) NOT NULL DEFAULT '',
  content TEXT NOT NULL,
  is_read TINYINT NOT NULL DEFAULT 0,
  is_claimed TINYINT NOT NULL DEFAULT 0,
  reward_json TEXT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_user_time (user_id, mail_time DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS user_progress (
  user_id BIGINT NOT NULL PRIMARY KEY,
  brave_cur INT NOT NULL DEFAULT 1,
  brave_max INT NOT NULL DEFAULT 1,
  fresh_cur INT NOT NULL DEFAULT 1,
  fresh_max INT NOT NULL DEFAULT 1,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS user_spt_defeated (
  user_id BIGINT NOT NULL,
  boss_key INT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, boss_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS user_nono (
  user_id BIGINT NOT NULL PRIMARY KEY,
  has_nono TINYINT NOT NULL DEFAULT 0,
  flag INT NOT NULL DEFAULT 1,
  state INT NOT NULL DEFAULT 0,
  nick VARCHAR(16) NOT NULL DEFAULT 'NoNo',
  color INT NOT NULL DEFAULT 16777215,
  super_nono INT NOT NULL DEFAULT 0,
  power INT NOT NULL DEFAULT 100,
  mate INT NOT NULL DEFAULT 100,
  iq INT NOT NULL DEFAULT 0,
  ai SMALLINT NOT NULL DEFAULT 0,
  birth BIGINT NOT NULL DEFAULT 0,
  charge_time INT NOT NULL DEFAULT 0,
  func_bits VARBINARY(20) NOT NULL DEFAULT '',
  super_energy INT NOT NULL DEFAULT 0,
  super_level INT NOT NULL DEFAULT 0,
  super_stage INT NOT NULL DEFAULT 0,
  super_months INT NOT NULL DEFAULT 0,
  vip_value INT NOT NULL DEFAULT 0,
  auto_charge INT NOT NULL DEFAULT 0,
  vip_end_time BIGINT NOT NULL DEFAULT 0,
  open_gold_charged TINYINT NOT NULL DEFAULT 0,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("schema: %w", err)
		}
	}
	if err := s.ensureAchieveSchema(); err != nil {
		return err
	}
	if err := s.ensurePetExtraSchema(); err != nil {
		return err
	}
	if err := s.ensureFitmentSchema(); err != nil {
		return err
	}
	if err := s.ensureBreedSchema(); err != nil {
		return err
	}
	// 已有库增量列（重复 ADD 忽略）
	for _, q := range []string{
		`ALTER TABLE user_mails ADD COLUMN is_claimed TINYINT NOT NULL DEFAULT 0`,
		`ALTER TABLE user_mails ADD COLUMN reward_json TEXT NULL`,
		`ALTER TABLE users ADD COLUMN exp_pool INT NOT NULL DEFAULT 0`,
		`ALTER TABLE user_progress ADD COLUMN recruit_mask INT NOT NULL DEFAULT 0`,
		`ALTER TABLE user_progress ADD COLUMN gaiya_def INT NOT NULL DEFAULT 0`,
		`ALTER TABLE user_progress ADD COLUMN gaiya_mask INT NOT NULL DEFAULT 0`,
		`ALTER TABLE user_progress ADD COLUMN two_times INT NOT NULL DEFAULT 0`,
		`ALTER TABLE user_progress ADD COLUMN three_times INT NOT NULL DEFAULT 0`,
		`ALTER TABLE user_progress ADD COLUMN auto_fight_times INT NOT NULL DEFAULT 0`,
		`ALTER TABLE user_progress ADD COLUMN energy_times INT NOT NULL DEFAULT 0`,
		`ALTER TABLE user_progress ADD COLUMN learn_times INT NOT NULL DEFAULT 0`,
		`ALTER TABLE user_progress ADD COLUMN auto_fight INT NOT NULL DEFAULT 0`,
	} {
		if _, err := s.db.Exec(q); err != nil && !isDupColumnErr(err) {
			return fmt.Errorf("schema migrate: %w", err)
		}
	}
	return nil
}

func isDupColumnErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Duplicate column") || strings.Contains(msg, "1060")
}

func (s *sqlBackend) FindByEmail(email string) (*User, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	u := &User{}
	var role int
	err := s.db.QueryRow(`
SELECT user_id,email,password,nickname,color,role_created,coins,gold,energy,exp_total,
       map_id,pos_x,pos_y,session_hex,last_online,login_cnt,register_time
FROM users WHERE email=?`, email).Scan(
		&u.UserID, &u.Email, &u.Password, &u.Nickname, &u.Color, &role,
		&u.Coins, &u.Gold, &u.Energy, &u.ExpTotal, &u.MapID, &u.PosX, &u.PosY,
		&u.SessionHex, &u.LastOnline, &u.LoginCnt, &u.RegisterTime,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.RoleCreated = role != 0
	return u, nil
}

func (s *sqlBackend) FindByUserID(uid int64) (*User, error) {
	u := &User{}
	var role int
	err := s.db.QueryRow(`
SELECT user_id,email,password,nickname,color,role_created,coins,gold,energy,exp_total,
       map_id,pos_x,pos_y,session_hex,last_online,login_cnt,register_time
FROM users WHERE user_id=?`, uid).Scan(
		&u.UserID, &u.Email, &u.Password, &u.Nickname, &u.Color, &role,
		&u.Coins, &u.Gold, &u.Energy, &u.ExpTotal, &u.MapID, &u.PosX, &u.PosY,
		&u.SessionHex, &u.LastOnline, &u.LoginCnt, &u.RegisterTime,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.RoleCreated = role != 0
	return u, nil
}

func (s *sqlBackend) CreateUser(email, passwordMD5 string) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	uid := s.nextID.Add(1)
	now := time.Now().Unix()
	u := &User{
		UserID:       uid,
		Email:        strings.TrimSpace(strings.ToLower(email)),
		Password:     passwordMD5,
		Nickname:     "",
		Energy:       100,
		MapID:        1,
		PosX:         475,
		PosY:         395,
		RegisterTime: now,
	}
	_, err := s.db.Exec(`
INSERT INTO users(user_id,email,password,nickname,color,role_created,coins,gold,energy,exp_total,
 map_id,pos_x,pos_y,session_hex,last_online,login_cnt,register_time)
VALUES(?,?,?,?,?,0,0,0,100,0,1,475,395,'',0,0,?)`,
		u.UserID, u.Email, u.Password, u.Nickname, u.Color, u.RegisterTime)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *sqlBackend) SaveUser(u *User) error {
	role := 0
	if u.RoleCreated {
		role = 1
	}
	_, err := s.db.Exec(`
UPDATE users SET email=?,password=?,nickname=?,color=?,role_created=?,coins=?,gold=?,energy=?,
 exp_total=?,map_id=?,pos_x=?,pos_y=?,session_hex=?,last_online=?,login_cnt=?,register_time=?
WHERE user_id=?`,
		u.Email, u.Password, u.Nickname, u.Color, role, u.Coins, u.Gold, u.Energy,
		u.ExpTotal, u.MapID, u.PosX, u.PosY, u.SessionHex, u.LastOnline, u.LoginCnt, u.RegisterTime,
		u.UserID)
	return err
}

func NewSession() (raw []byte, hexStr string, err error) {
	raw = make([]byte, 16)
	if _, err = rand.Read(raw); err != nil {
		return nil, "", err
	}
	return raw, hex.EncodeToString(raw), nil
}

func DSN(host string, port int, user, pass, db string) string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		user, pass, host, port, db)
}
