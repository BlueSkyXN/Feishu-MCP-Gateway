// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// guidance 由 scripts/import_catalog.py 从固定 lark-cli 的 affordance、shortcut tips 与 Skill 文档生成。
// 它只影响 Agent 看到的说明，不改变目录、权限或执行映射，因此不计入 catalog_revision。
//
//go:embed guidance
var guidanceFS embed.FS

type GuidanceExample struct {
	Description string         `json:"description,omitempty"`
	OperationID string         `json:"operation_id"`
	Arguments   map[string]any `json:"arguments"`
}

type OperationGuidance struct {
	UseWhen       []string          `json:"use_when,omitempty"`
	AvoidWhen     []string          `json:"avoid_when,omitempty"`
	Prerequisites []string          `json:"prerequisites,omitempty"`
	Tips          []string          `json:"tips,omitempty"`
	Examples      []GuidanceExample `json:"examples,omitempty"`
	Related       []string          `json:"related,omitempty"`
	Topics        []string          `json:"topics,omitempty"`
}

type GuideTopic struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type GuideSkill struct {
	Domain      string       `json:"domain"`
	Description string       `json:"description"`
	Topics      []GuideTopic `json:"topics"`
}

type Guidance struct {
	SourceCommit string                       `json:"source_commit"`
	Skills       map[string]GuideSkill        `json:"skills"`
	Operations   map[string]OperationGuidance `json:"operations"`
	topicSkill   map[string]string
	topicTitle   map[string]string
	revision     string
}

// GuidanceRevision 是全部指南文件的摘要，便于核对部署中实际提供给 Agent 的说明版本。
func GuidanceRevision() string {
	g, err := loadGuidanceFiles()
	if err != nil {
		return ""
	}
	return g.revision
}

func loadGuidanceFiles() (*Guidance, error) {
	paths := []string{}
	if err := fs.WalkDir(guidanceFS, "guidance", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			paths = append(paths, path)
		}
		return err
	}); err != nil {
		return nil, err
	}
	sort.Strings(paths)
	sum := []byte{}
	for _, path := range paths {
		b, err := guidanceFS.ReadFile(path)
		if err != nil {
			return nil, err
		}
		sum = append(sum, []byte(path+"\x00"+digest(b)+"\n")...)
	}
	raw, err := guidanceFS.ReadFile("guidance/operations.json")
	if err != nil {
		return nil, err
	}
	var g Guidance
	if err = json.Unmarshal(raw, &g); err != nil {
		return nil, err
	}
	g.revision = digest(sum)
	return &g, nil
}

// LoadGuidance 读取指南并与目录交叉校验：引用的操作、主题和领域都必须存在。
func LoadGuidance(c *Catalog) (*Guidance, error) {
	g, err := loadGuidanceFiles()
	if err != nil {
		return nil, err
	}
	if g.SourceCommit != c.SourceCommit {
		return nil, fmt.Errorf("指南来源 %s 与目录来源 %s 不一致", g.SourceCommit, c.SourceCommit)
	}
	g.topicSkill, g.topicTitle = map[string]string{}, map[string]string{}
	for name, skill := range g.Skills {
		if _, ok := domainByID(skill.Domain); !ok {
			return nil, fmt.Errorf("指南 %s 指向未知领域 %s", name, skill.Domain)
		}
		if _, err := guidanceFS.ReadFile("guidance/" + name + "/GUIDE.md"); err != nil {
			return nil, fmt.Errorf("指南 %s 缺少 GUIDE.md", name)
		}
		for _, t := range skill.Topics {
			if _, dup := g.topicSkill[t.ID]; dup {
				return nil, fmt.Errorf("指南主题重名: %s", t.ID)
			}
			if _, err := guidanceFS.ReadFile("guidance/" + name + "/" + t.ID + ".md"); err != nil {
				return nil, fmt.Errorf("指南主题缺少文件: %s", t.ID)
			}
			g.topicSkill[t.ID], g.topicTitle[t.ID] = name, t.Title
		}
	}
	for _, d := range domains {
		if _, ok := g.Skills[d.Skill]; d.Skill != "" && !ok {
			return nil, fmt.Errorf("领域 %s 的指南 %s 未导入", d.ID, d.Skill)
		}
		for _, id := range d.Featured {
			if op, ok := c.Operation(id); !ok || opDomain(op) != d.ID {
				return nil, fmt.Errorf("领域 %s 的入口操作无效: %s", d.ID, id)
			}
		}
	}
	for id, og := range g.Operations {
		if _, ok := c.Operation(id); !ok {
			return nil, fmt.Errorf("指南引用了未登记的操作: %s", id)
		}
		for _, ref := range og.Related {
			if _, ok := c.Operation(ref); !ok {
				return nil, fmt.Errorf("指南 %s 的相关操作未登记: %s", id, ref)
			}
		}
		for _, ex := range og.Examples {
			if _, ok := c.Operation(ex.OperationID); !ok {
				return nil, fmt.Errorf("指南 %s 的示例操作未登记: %s", id, ex.OperationID)
			}
		}
		for _, topic := range og.Topics {
			if _, ok := g.topicSkill[topic]; !ok {
				return nil, fmt.Errorf("指南 %s 引用了未知主题: %s", id, topic)
			}
		}
	}
	for _, op := range c.Operations {
		if _, ok := domainByID(opDomain(op)); !ok {
			return nil, fmt.Errorf("操作 %s 不属于任何已登记领域", op.ID)
		}
	}
	return g, nil
}

func (g *Guidance) guide(skill string) (string, bool) {
	b, err := guidanceFS.ReadFile("guidance/" + skill + "/GUIDE.md")
	return string(b), err == nil
}

func (g *Guidance) topic(id string) (skill, title, content string, ok bool) {
	skill, ok = g.topicSkill[id]
	if !ok {
		return "", "", "", false
	}
	b, err := guidanceFS.ReadFile("guidance/" + skill + "/" + id + ".md")
	return skill, g.topicTitle[id], string(b), err == nil
}

// firstSentence 截取说明的第一句，用于工具描述等需要简短的位置。
func firstSentence(s string, limit int) string {
	s = strings.TrimSpace(s)
	for _, sep := range []string{"。", ". ", "；", "; "} {
		if i := strings.Index(s, sep); i > 0 {
			s = s[:i+len(strings.TrimSpace(sep))]
			break
		}
	}
	return truncateRunes(s, limit)
}

func truncateRunes(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return strings.TrimSpace(string(r[:limit-1])) + "…"
}
