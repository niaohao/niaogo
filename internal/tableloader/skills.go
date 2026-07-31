package tableloader

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// SkillDef 对齐 SkillXMLInfo.xml 的 Move 常用字段。
type SkillDef struct {
	ID            int
	Name          string
	Category      int // 1物 2特 4变化
	Type          int
	Power         int
	MaxPP         int
	Accuracy      int
	MustHit       int // 1=必中
	SideEffect    string
	SideEffectArg string
}

func (c *Catalog) Skill(id int) *SkillDef {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.Skills == nil {
		return nil
	}
	if s, ok := c.Skills[id]; ok {
		cp := s
		return &cp
	}
	return nil
}

func (c *Catalog) SkillCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.Skills)
}

func (c *Catalog) loadSkills(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read skills: %w", err)
	}
	type moveNode struct {
		ID            string `xml:"ID,attr"`
		Name          string `xml:"Name,attr"`
		Category      string `xml:"Category,attr"`
		Type          string `xml:"Type,attr"`
		Power         string `xml:"Power,attr"`
		MaxPP         string `xml:"MaxPP,attr"`
		Accuracy      string `xml:"Accuracy,attr"`
		MustHit       string `xml:"MustHit,attr"`
		SideEffect    string `xml:"SideEffect,attr"`
		SideEffectArg string `xml:"SideEffectArg,attr"`
	}
	type root struct {
		Moves []moveNode `xml:"Moves>Move"`
	}
	// 兼容扁平 <Move> 与 <MovesTbl><Moves><Move>…
	type flatRoot struct {
		Moves []moveNode `xml:"Move"`
	}
	var r root
	if err := xml.Unmarshal(b, &r); err != nil {
		return fmt.Errorf("parse skills: %w", err)
	}
	if len(r.Moves) == 0 {
		var fr flatRoot
		_ = xml.Unmarshal(b, &fr)
		r.Moves = fr.Moves
	}
	if c.Skills == nil {
		c.Skills = make(map[int]SkillDef, len(r.Moves))
	}
	for _, m := range r.Moves {
		id, err := strconv.Atoi(m.ID)
		if err != nil || id <= 0 {
			continue
		}
		pp, _ := strconv.Atoi(m.MaxPP)
		if pp <= 0 {
			pp = 20
		}
		c.Skills[id] = SkillDef{
			ID:            id,
			Name:          m.Name,
			Category:      atoiDefault(m.Category, 1),
			Type:          atoiDefault(m.Type, 8),
			Power:         atoiDefault(m.Power, 0),
			MaxPP:         pp,
			Accuracy:      atoiDefault(m.Accuracy, 100),
			MustHit:       atoiDefault(m.MustHit, 0),
			SideEffect:    strings.TrimSpace(m.SideEffect),
			SideEffectArg: strings.TrimSpace(m.SideEffectArg),
		}
	}
	return nil
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

// ensure Load also picks SkillXMLInfo.xml（文件名以仓库为准）。
func skillXMLPath(xmlDir string) string {
	cand := []string{
		filepath.Join(xmlDir, "SkillXMLInfo.xml"),
		filepath.Join(xmlDir, "skills.xml"),
	}
	for _, p := range cand {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return cand[0]
}
