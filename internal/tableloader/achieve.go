package tableloader

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// AchieveRule 成就分支内一条 Rule（AchieveXMLInfo.xml）。
type AchieveRule struct {
	BranchID         int
	RuleID           int
	Threshold        int
	AchievementPoint int
	SpeNameBonus     int
	Title            string
	SourceKey        string
}

// AchieveBranch 成就分支。
type AchieveBranch struct {
	ID   int
	Type int // XML @type：2=战斗类等
	Desc string
	Rules []AchieveRule
}

type achieveXMLRoot struct {
	Types []achieveXMLType `xml:"type"`
}

type achieveXMLType struct {
	Branches []achieveXMLBranches `xml:"Branches"`
}

type achieveXMLBranches struct {
	Branch []achieveXMLBranch `xml:"Branch"`
}

type achieveXMLBranch struct {
	ID    string           `xml:"ID,attr"`
	Type  string           `xml:"type,attr"`
	Desc  string           `xml:"Desc,attr"`
	Rules []achieveXMLRule `xml:"Rule"`
}

type achieveXMLRule struct {
	ID               string `xml:"ID,attr"`
	Threshold        string `xml:"Threshold,attr"`
	AchievementPoint string `xml:"AchievementPoint,attr"`
	SpeNameBonus     string `xml:"SpeNameBonus,attr"`
	Title            string `xml:"title,attr"`
	SourceKey        string `xml:"SourceKey,attr"`
}

func (c *Catalog) loadAchieve(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var root achieveXMLRoot
	if err := xml.Unmarshal(b, &root); err != nil {
		return fmt.Errorf("AchieveXML: %w", err)
	}
	byBranch := make(map[int]*AchieveBranch)
	rulesByKey := make(map[int]AchieveRule) // key = branch*100+rule
	order := make([]int, 0)
	for _, typ := range root.Types {
		for _, grp := range typ.Branches {
			for _, xb := range grp.Branch {
				bid, _ := strconv.Atoi(xb.ID)
				if bid <= 0 {
					continue
				}
				br := &AchieveBranch{
					ID:   bid,
					Type: atoiDefault(xb.Type, 0),
					Desc: xb.Desc,
				}
				for _, xr := range xb.Rules {
					rid, _ := strconv.Atoi(xr.ID)
					if rid <= 0 {
						continue
					}
					r := AchieveRule{
						BranchID:         bid,
						RuleID:           rid,
						Threshold:        atoiDefault(xr.Threshold, 0),
						AchievementPoint: atoiDefault(xr.AchievementPoint, 0),
						SpeNameBonus:     atoiDefault(xr.SpeNameBonus, 0),
						Title:            xr.Title,
						SourceKey:        xr.SourceKey,
					}
					br.Rules = append(br.Rules, r)
					rulesByKey[achieveRuleKey(bid, rid)] = r
				}
				byBranch[bid] = br
				order = append(order, bid)
			}
		}
	}
	c.AchieveByBranch = byBranch
	c.AchieveRules = rulesByKey
	c.AchieveBranchOrder = order
	fmt.Printf("[tables] achieve branches=%d rules=%d\n", len(byBranch), len(rulesByKey))
	return nil
}

func achieveRuleKey(branchID, ruleID int) int {
	return branchID*100 + ruleID
}

// AchieveBranchOf 取分支；不存在返回 nil。
func (c *Catalog) AchieveBranchOf(branchID int) *AchieveBranch {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.AchieveByBranch[branchID]
}

// AchieveRulesOfBranch 分支下全部 Rule。
func (c *Catalog) AchieveRulesOfBranch(branchID int) []AchieveRule {
	br := c.AchieveBranchOf(branchID)
	if br == nil {
		return nil
	}
	out := make([]AchieveRule, len(br.Rules))
	copy(out, br.Rules)
	return out
}

// AchieveRuleOf key=branch*100+rule。
func (c *Catalog) AchieveRuleOf(branchID, ruleID int) (AchieveRule, bool) {
	if c == nil {
		return AchieveRule{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	r, ok := c.AchieveRules[achieveRuleKey(branchID, ruleID)]
	return r, ok
}

// AchieveIsBitmaskBranch 客户端以位图解析 value 的分支（AchieveCmdListener）。
func AchieveIsBitmaskBranch(branchID int) bool {
	switch branchID {
	case 2, 14, 21, 58, 64, 103, 104, 105, 106, 108, 110:
		return true
	default:
		return false
	}
}

// AchieveCompleteBit 完成位：分支2 见客户端 creatList 特例，其余 ruleID-1。
func AchieveCompleteBit(branchID, ruleID int) int {
	if ruleID <= 0 || ruleID > 32 {
		return -1
	}
	if branchID == 2 {
		if ruleID >= 16 {
			return ruleID
		}
		return ruleID - 1
	}
	return ruleID - 1
}

func achieveXMLPath(xmlDir string) string {
	return filepath.Join(xmlDir, "AchieveXMLInfo.xml")
}
