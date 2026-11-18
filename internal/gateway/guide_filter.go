// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"regexp"
	"strings"
)

var guideOperationRef = regexp.MustCompile(`\b[a-z][a-z0-9_]*(?:\.\+?[a-z][a-z0-9_-]*)+\b|\bfeishu_[a-z0-9_]+\b`)
var guideTopicRef = regexp.MustCompile("主题\\s*`([^`]+)`|topic\\s*=\\s*\"([^\"]+)\"")
var guideHeading = regexp.MustCompile(`^(#{1,6})[ \t]+`)
var guideTableDivider = regexp.MustCompile(`^\s*\|(?:\s*:?-+:?\s*\|)+\s*$`)

type guideSection struct {
	level   int
	heading string
	blocks  []string
}

type guideDocument struct {
	sections []guideSection
}

// 从内嵌的生成文件建立只读索引；每次请求只筛选视图，不改写共享指南。
func (g *Guidance) prepareDocuments(c *Catalog) {
	g.documents = map[string]guideDocument{}
	g.topicOwners = map[string][]string{}
	g.toolOperations = map[string]string{}
	for _, op := range c.Operations {
		g.toolOperations[op.Tool] = op.ID
	}
	for id, operation := range g.Operations {
		for _, topic := range operation.Topics {
			g.topicOwners[topic] = append(g.topicOwners[topic], id)
		}
	}
	for skill, value := range g.Skills {
		content, _ := g.guide(skill)
		g.documents[skill] = parseGuideDocument(content)
		for _, topic := range value.Topics {
			_, _, content, _ := g.topic(topic.ID)
			g.documents[topic.ID] = parseGuideDocument(content)
			// 标题点名的操作是主题主体；正文中的回查、后续操作不能反向授权主题。
			if owners := g.operationRefs(topic.Title, c); len(owners) > 0 {
				g.topicOwners[topic.ID] = owners
			}
		}
	}
}

func (g *Guidance) operationRefs(text string, c *Catalog) []string {
	refs := []string{}
	for _, ref := range guideOperationRef.FindAllString(text, -1) {
		if id, ok := g.toolOperations[ref]; ok {
			ref = id
		}
		if _, ok := c.Operation(ref); ok || strings.Contains(ref, ".+") {
			refs = append(refs, ref)
		}
	}
	return refs
}

type guidanceView struct {
	guidance   *Guidance
	catalog    *Catalog
	operations map[string]bool
	topics     map[string]bool
}

func (s *Service) guidanceView(c Client, p Principal) guidanceView {
	v := guidanceView{s.Guidance, s.Catalog, map[string]bool{}, map[string]bool{}}
	skills := map[string]bool{}
	for _, op := range s.Catalog.Operations {
		if discoverable(c, p, op) {
			v.operations[op.ID] = true
			domain, _ := domainByID(opDomain(op))
			skills[domain.Skill] = true
		}
	}
	for topic, skill := range s.Guidance.topicSkill {
		if !skills[skill] {
			continue
		}
		owners := s.Guidance.topicOwners[topic]
		allowed := len(owners) == 0
		for _, id := range owners {
			allowed = allowed || v.operations[id]
		}
		// 标题中的多个操作视为一个完整主题，避免展示只获准一部分的流程。
		for _, id := range s.Guidance.operationRefs(s.Guidance.topicTitle[topic], s.Catalog) {
			allowed = allowed && v.operations[id]
		}
		v.topics[topic] = allowed
	}
	return v
}

func (v guidanceView) allowsText(text string) bool {
	for _, id := range v.guidance.operationRefs(text, v.catalog) {
		if !v.operations[id] {
			return false
		}
	}
	for _, match := range guideTopicRef.FindAllStringSubmatch(text, -1) {
		id := match[1] + match[2]
		if !v.topics[id] {
			return false
		}
	}
	// 无法等价转换的 CLI 片段不能作为当前权限下的 MCP 调用说明。
	return !strings.Contains(text, "lark-cli ")
}

func (v guidanceView) filterLines(lines []string) []string {
	out := []string{}
	for _, line := range lines {
		if v.allowsText(line) {
			out = append(out, line)
		}
	}
	return out
}

func (v guidanceView) skillTopics(skill string) []GuideTopic {
	out := []GuideTopic{}
	for _, topic := range v.guidance.Skills[skill].Topics {
		if v.topics[topic.ID] {
			out = append(out, topic)
		}
	}
	return out
}

func (v guidanceView) content(id string) string {
	var out strings.Builder
	blockedLevel := 0
	for _, section := range v.guidance.documents[id].sections {
		if blockedLevel > 0 && section.level > blockedLevel {
			continue
		}
		blockedLevel = 0
		if !v.allowsText(section.heading) {
			blockedLevel = section.level
			continue
		}
		out.WriteString(section.heading + "\n\n")
		for _, block := range section.blocks {
			if filtered := v.filterBlock(block); filtered != "" {
				out.WriteString(filtered + "\n\n")
			}
		}
	}
	return strings.TrimSpace(out.String()) + "\n"
}

func (v guidanceView) filterBlock(block string) string {
	if v.allowsText(block) {
		return block
	}
	lines := strings.Split(block, "\n")
	if len(lines) < 3 || !strings.HasPrefix(strings.TrimSpace(lines[0]), "|") || !guideTableDivider.MatchString(lines[1]) || !v.allowsText(lines[0]) {
		return ""
	}
	// 表格行各自描述一个条目，保留合法行及完整表头；段落、列表和代码块仍整体筛选。
	kept := append([]string{}, lines[:2]...)
	for _, row := range lines[2:] {
		if !strings.HasPrefix(strings.TrimSpace(row), "|") {
			return ""
		}
		if v.allowsText(row) {
			kept = append(kept, row)
		}
	}
	if len(kept) == 2 {
		return ""
	}
	return strings.Join(kept, "\n")
}

// 按标题与完整段落、列表、表格、代码块分段；筛选时表格可以保留独立的合法行。
// 自动生成的 CLI 前言不参与正文；调用方式由当前工具视图单独提供。
func parseGuideDocument(content string) guideDocument {
	doc := guideDocument{}
	var current *guideSection
	block := []string{}
	fence := ""
	flush := func() {
		if current != nil && len(block) > 0 {
			current.blocks = append(current.blocks, strings.Join(block, "\n"))
		}
		block = nil
	}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		fenceLine := trimmed
		for strings.HasPrefix(fenceLine, ">") {
			fenceLine = strings.TrimSpace(strings.TrimPrefix(fenceLine, ">"))
		}
		if fence == "" {
			if heading := guideHeading.FindStringSubmatch(line); len(heading) > 0 {
				flush()
				doc.sections = append(doc.sections, guideSection{level: len(heading[1]), heading: line})
				current = &doc.sections[len(doc.sections)-1]
				continue
			}
			if trimmed == "" {
				flush()
				continue
			}
			if strings.HasPrefix(fenceLine, "```") || strings.HasPrefix(fenceLine, "~~~") {
				fence = fenceLine[:len(fenceLine)-len(strings.TrimLeft(fenceLine, fenceLine[:1]))]
			}
		} else if strings.HasPrefix(fenceLine, fence) && strings.Trim(fenceLine, fence[:1]+" \t") == "" {
			fence = ""
		}
		if current != nil {
			block = append(block, line)
		}
	}
	flush()
	return doc
}
