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
	return strings.NewReplacer("{{DOMAINS}}", strings.Join(titles, "、"), "{{ROUTES}}", strings.Join(rows, "\n")).Replace(skillTemplate)
}
