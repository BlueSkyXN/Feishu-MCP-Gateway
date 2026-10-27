// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"sort"
	"strings"
	"unicode"
)

// discoverable 判断操作对当前请求是否可发现，与执行授权共用 operationAllowed。
func discoverable(c Client, p Principal, op Operation) bool {
	return operationAllowed(c, p, op) == nil
}

type domainView struct {
	Domain     string   `json:"domain"`
	Title      string   `json:"title"`
	Summary    string   `json:"summary,omitempty"`
	Operations int      `json:"operations"`
	Read       int      `json:"read"`
	Write      int      `json:"write"`
	Featured   []string `json:"featured,omitempty"`
	Guide      bool     `json:"guide"`
	Topics     int      `json:"topics,omitempty"`
}

// visibleDomains 按领域表顺序统计当前请求可发现的操作。
func (s *Service) visibleDomains(c Client, p Principal) []domainView {
	counts := map[string]*domainView{}
	for _, op := range s.Catalog.Operations {
		if !discoverable(c, p, op) {
			continue
		}
		d := counts[opDomain(op)]
		if d == nil {
			d = &domainView{Domain: opDomain(op)}
			counts[d.Domain] = d
		}
		d.Operations++
		if op.Risk == "read" {
			d.Read++
		} else {
			d.Write++
		}
	}
	out := []domainView{}
	for _, meta := range domains {
		d := counts[meta.ID]
		if d == nil {
			continue
		}
		d.Title, d.Summary = meta.Title, meta.Summary
		for _, id := range meta.Featured {
			if op, ok := s.Catalog.Operation(id); ok && discoverable(c, p, op) {
				d.Featured = append(d.Featured, id)
			}
		}
		if skill, ok := s.Guidance.Skills[meta.Skill]; ok {
			d.Guide, d.Topics = true, len(skill.Topics)
		}
		out = append(out, *d)
	}
	return out
}

// searchVocabulary 把中文与常见英文说法归一到目录中的英文词；领域别名另由领域表加入。
var searchVocabulary = map[string][]string{
	"读取": {"get", "fetch", "read", "list", "query"}, "读": {"get", "fetch", "read", "list"}, "阅读": {"get", "fetch", "read"},
	"获取": {"get", "fetch", "list"}, "查看": {"get", "fetch", "list", "info", "detail"}, "看": {"get", "fetch", "list"},
	"查询": {"query", "get", "list", "search"}, "查": {"query", "get", "list", "search"}, "列出": {"list"}, "列表": {"list"},
	"搜索": {"search", "find"}, "查找": {"search", "find"}, "检索": {"search"},
	"创建": {"create", "add"}, "新建": {"create", "add"}, "新增": {"create", "add", "append"}, "建": {"create"}, "添加": {"add", "create"},
	"更新": {"update", "patch", "edit", "set"}, "修改": {"update", "patch", "edit", "set"}, "改": {"update", "edit"}, "编辑": {"edit", "update"},
	"写入": {"set", "put", "write", "update", "append"}, "写": {"set", "put", "write"}, "填写": {"set", "put"}, "追加": {"append", "add"},
	"删除": {"delete", "remove"}, "删": {"delete", "remove"}, "移除": {"remove", "delete"}, "清空": {"clear"},
	"发送": {"send"}, "发": {"send"}, "回复": {"reply"}, "转发": {"forward"}, "撤回": {"recall", "delete"},
	"上传": {"upload"}, "下载": {"download"}, "导出": {"export"}, "导入": {"import"}, "移动": {"move"}, "复制": {"copy"},
	"分享": {"share"}, "合并": {"merge"}, "排序": {"sort"}, "筛选": {"filter"}, "过滤": {"filter"}, "替换": {"replace"},
	"回滚": {"revert", "history"}, "完成": {"complete"}, "分配": {"assign"}, "邀请": {"invite"}, "预订": {"room", "book"},
	"消息": {"message", "messages"}, "记录": {"record", "records"}, "群": {"chat", "chats"}, "群聊": {"chat", "chats"}, "文档": {"doc", "docs", "document"},
	"文件": {"file", "files"}, "图片": {"image", "images"}, "附件": {"attachment", "attachments", "file"}, "成员": {"member", "members"},
	"公式": {"formula"}, "样式": {"style", "styles"}, "图表": {"chart", "charts"}, "透视表": {"pivot"}, "下拉": {"dropdown"},
	"字段": {"field", "fields"}, "视图": {"view", "views"}, "表": {"table", "tables"}, "仪表盘": {"dashboard"}, "表单": {"form"},
	"评论": {"comment", "comments"}, "权限": {"permission", "permissions", "member"}, "版本": {"history", "revision", "version"},
	"历史": {"history"}, "封面": {"cover"}, "标题": {"title"}, "链接": {"url", "link"}, "用户": {"user", "users"},
	"参会人": {"attendee", "attendees"}, "子任务": {"subtask"}, "草稿": {"draft", "drafts"}, "标签": {"label", "labels"},
	"规则": {"rule", "rules"}, "周期": {"cycle"}, "进展": {"progress"}, "页面": {"slide", "page"}, "截图": {"screenshot"},
	"read": {"get", "fetch", "read", "list"}, "view": {"get", "list", "fetch"}, "show": {"get", "list", "fetch"}, "see": {"get", "list"},
	"lookup": {"get", "search"}, "find": {"search", "find"}, "write": {"set", "put", "write", "update"}, "fill": {"set", "put"},
	"modify": {"update", "edit", "patch"}, "change": {"update", "edit", "patch"}, "edit": {"edit", "update"}, "new": {"create"},
	"add": {"add", "create", "append"}, "remove": {"remove", "delete"}, "post": {"send", "create"}, "rollback": {"revert"},
	"msg": {"message", "messages"}, "picture": {"image"}, "photo": {"image"}, "attachment": {"attachment", "file"},
	"workbook": {"workbook", "domain:sheets"}, "table": {"table", "tables"}, "group": {"chat"}, "share": {"share", "permission"},
}

type concept struct {
	alts []string
	soft bool // 未登记的中文片段：命中加分，但不作为必须条件
}

type searchEntry struct {
	op       Operation
	domain   string
	words    map[string]bool
	pack     map[string]bool
	params   map[string]bool
	text     string
	textSet  map[string]bool
	featured int
}

type searchIndex struct {
	entries []*searchEntry
	terms   map[string][]string
	keys    []string
}

func splitWords(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)))
	})
}

func wordSet(values ...string) map[string]bool {
	out := map[string]bool{}
	for _, v := range values {
		for _, w := range splitWords(v) {
			out[w] = true
		}
	}
	return out
}

func newSearchIndex(c *Catalog, g *Guidance) *searchIndex {
	x := &searchIndex{terms: map[string][]string{}}
	for k, v := range searchVocabulary {
		x.terms[k] = append(x.terms[k], v...)
	}
	featured := map[string]int{}
	for _, d := range domains {
		for _, a := range append([]string{d.ID}, d.Aliases...) {
			x.terms[a] = append(x.terms[a], "domain:"+d.ID)
		}
		for i, id := range d.Featured {
			featured[id] = i + 1
		}
	}
	for k := range x.terms {
		if hasHan(k) {
			x.keys = append(x.keys, k)
		}
	}
	// 中文按最长词条优先切分。
	sort.Slice(x.keys, func(i, j int) bool {
		if a, b := len([]rune(x.keys[i])), len([]rune(x.keys[j])); a != b {
			return a > b
		}
		return x.keys[i] < x.keys[j]
	})
	for _, op := range c.Operations {
		params := []string{}
		for name := range object(op.InputSchema["properties"]) {
			if name != "async" && name != "workspace_files" && name != "idempotency_key" {
				params = append(params, name, strings.ReplaceAll(name, "_", " "))
			}
		}
		text := op.Description
		if og, ok := g.Operations[op.ID]; ok && len(og.UseWhen) > 0 {
			text += " " + strings.Join(og.UseWhen, " ")
		}
		text = strings.ToLower(text)
		x.entries = append(x.entries, &searchEntry{op: op, domain: opDomain(op), words: wordSet(op.ID, op.Tool), pack: wordSet(op.Pack), params: wordSet(params...), text: text, textSet: wordSet(text), featured: featured[op.ID]})
	}
	return x
}

func hasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func (x *searchIndex) expand(word string) concept {
	alts := []string{word}
	alts = append(alts, x.terms[word]...)
	if singular := strings.TrimSuffix(word, "s"); singular != word && len(singular) > 2 {
		alts = append(alts, singular)
		alts = append(alts, x.terms[singular]...)
	}
	return concept{alts: alts}
}

// parse 把查询拆成若干概念：每个概念内的词可互换，概念之间需要同时命中。
func (x *searchIndex) parse(query string) []concept {
	out := []concept{}
	for _, field := range strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(",，、;；/|()（）[]【】\"'“”‘’:：!！?？。", r)
	}) {
		if !hasHan(field) {
			for _, w := range splitWords(field) {
				out = append(out, x.expand(w))
			}
			continue
		}
		runes, rest := []rune(field), []rune{}
		flush := func() {
			if len(rest) == 0 {
				return
			}
			piece := string(rest)
			rest = rest[:0]
			if hasHan(piece) {
				out = append(out, concept{alts: []string{piece}, soft: true})
				return
			}
			for _, w := range splitWords(piece) {
				out = append(out, x.expand(w))
			}
		}
		for i := 0; i < len(runes); {
			matched := ""
			for _, key := range x.keys {
				k := []rune(key)
				if i+len(k) <= len(runes) && string(runes[i:i+len(k)]) == key {
					matched = key
					break
				}
			}
			if matched == "" {
				rest = append(rest, runes[i])
				i++
				continue
			}
			flush()
			out = append(out, x.expand(matched))
			i += len([]rune(matched))
		}
		flush()
	}
	return out
}

func boostIf(ok bool, v float64) float64 {
	if ok {
		return v
	}
	return 0
}

func (e *searchEntry) weight(alt string) float64 {
	switch {
	case strings.HasPrefix(alt, "domain:"):
		if alt[len("domain:"):] == e.domain {
			return 3
		}
		return 0
	case e.words[alt]:
		return 4
	case e.pack[alt]:
		return 2
	case e.params[alt]:
		return 1.5
	case e.textSet[alt], hasHan(alt) && strings.Contains(e.text, alt):
		return 1
	}
	return 0
}

type searchHit struct {
	entry   *searchEntry
	matched int
	score   float64
	boost   float64 // 入口操作加分，只在全部条件命中时参与排序
}

// search 返回排序后的命中；filter 决定可发现范围。全部必需概念命中的结果优先，没有时退回部分命中。
func (x *searchIndex) search(query string, filter func(Operation) bool) (hits []searchHit, partial bool) {
	query = strings.TrimSpace(query)
	for _, e := range x.entries {
		if (e.op.ID == query || e.op.Tool == query) && filter(e.op) {
			return []searchHit{{entry: e}}, false
		}
	}
	concepts := x.parse(query)
	required := 0
	for _, c := range concepts {
		if !c.soft {
			required++
		}
	}
	for _, e := range x.entries {
		if !filter(e.op) {
			continue
		}
		h := searchHit{entry: e}
		for _, c := range concepts {
			best := 0.0
			for _, alt := range c.alts {
				best = max(best, e.weight(alt))
			}
			if best > 0 && (!c.soft || required == 0) {
				h.matched++
			}
			h.score += best
		}
		if len(concepts) > 0 && h.matched == 0 {
			continue
		}
		if e.featured > 0 {
			h.boost = 2
		}
		if e.op.Kind == "shortcut" {
			h.score += 0.5
		}
		if e.op.Hidden {
			h.score -= 3
		}
		hits = append(hits, h)
	}
	need := required
	if need == 0 {
		need = len(concepts)
	}
	full := hits[:0:0]
	for _, h := range hits {
		if h.matched >= need {
			full = append(full, h)
		}
	}
	if len(full) > 0 && len(concepts) > 0 {
		hits = full
	} else if len(concepts) > 0 {
		partial = len(hits) > 0
	}
	order := map[string]int{}
	for i, d := range domains {
		order[d.ID] = i
	}
	sort.SliceStable(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		if len(concepts) == 0 {
			if order[a.entry.domain] != order[b.entry.domain] {
				return order[a.entry.domain] < order[b.entry.domain]
			}
		} else if a.matched != b.matched {
			return a.matched > b.matched
		} else if sa, sb := a.score+boostIf(!partial, a.boost), b.score+boostIf(!partial, b.boost); sa != sb {
			return sa > sb
		}
		if fa, fb := a.entry.featured, b.entry.featured; (fa > 0) != (fb > 0) || (fa > 0 && fa != fb) {
			return fa > 0 && (fb == 0 || fa < fb)
		}
		return a.entry.op.Tool < b.entry.op.Tool
	})
	return hits, partial
}
