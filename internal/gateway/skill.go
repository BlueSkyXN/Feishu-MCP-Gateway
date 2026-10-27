// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed skill_template.md
var skillTemplate string

// RenderSkill 生成随仓库发布的 feishu-gateway Skill。路由表来自领域表，与 feishu_guide、
// instructions 使用同一份领域数据；修改模板或领域表后运行 --render-skill 更新 skills/feishu-gateway/SKILL.md。
func RenderSkill() string {
	titles, rows := []string{}, []string{}
	for _, d := range domains {
		titles = append(titles, d.Title)
		featured := []string{}
		for _, id := range d.Featured {
			featured = append(featured, "`"+id+"`")
		}
		rows = append(rows, fmt.Sprintf("| %s `%s` | %s | %s |", d.Title, d.ID, d.Summary, strings.Join(featured, "、")))
	}
	return strings.NewReplacer("{{DOMAINS}}", strings.Join(titles, "、"), "{{ROUTES}}", strings.Join(rows, "\n"), "{{REFERENCES}}", referenceDirs()).Replace(skillTemplate)
}

// referenceDirs 说明离线指南目录：多数领域为 references/lark-<领域 ID>/，这里列出例外和没有指南的领域。
func referenceDirs() string {
	bySkill, order, none := map[string][]string{}, []string{}, []string{}
	for _, d := range domains {
		switch {
		case d.Skill == "":
			none = append(none, d.ID)
		case d.Skill != "lark-"+d.ID:
			if _, ok := bySkill[d.Skill]; !ok {
				order = append(order, d.Skill)
			}
			bySkill[d.Skill] = append(bySkill[d.Skill], d.ID)
		}
	}
	parts := []string{}
	for _, skill := range order {
		parts = append(parts, strings.Join(bySkill[skill], "、")+" 对应 `references/"+skill+"/`")
	}
	if len(none) > 0 {
		parts = append(parts, strings.Join(none, "、")+" 没有离线指南，用 `feishu_guide` 查看操作列表")
	}
	return strings.Join(append(parts, "其余领域对应 `references/lark-<领域 ID>/`"), "，")
}
