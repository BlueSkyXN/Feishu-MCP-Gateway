// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

// Package ingress normalizes optional caller metadata and selects configured
// application routes. Caller metadata does not authenticate a gateway client or
// grant a personal Feishu identity.
package ingress

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalidCaller  = errors.New("invalid caller metadata")
	ErrInvalidAdapter = errors.New("invalid caller adapter")
)

const (
	maxIDBytes      = 128
	maxLabelBytes   = 64
	maxNameBytes    = 1024
	maxHeaderBytes  = 128
	maxRawNameBytes = 4 * maxNameBytes
)

// Caller contains metadata supplied by the configured ingress adapter. ID is
// the primary identifier. OpenID and UnionID are supplemental exact-match
// attributes; they are never merged with ID or used to infer another identity.
type Caller struct {
	Source  string `json:"source,omitempty"`
	Channel string `json:"channel,omitempty"`
	ID      string `json:"id,omitempty"`
	IDType  string `json:"id_type,omitempty"`
	Name    string `json:"name,omitempty"`
	OpenID  string `json:"open_id,omitempty"`
	UnionID string `json:"union_id,omitempty"`
}

// ActorKey returns a stable primary-identity key. An explicit IDType takes
// precedence; known channel defaults preserve the existing Wake actor keys.
// Display names and supplemental identifiers never participate in this key.
func (c Caller) ActorKey() string {
	if !validIdentifier(c.Channel, maxLabelBytes) || !validIdentifier(c.ID, maxIDBytes) {
		return ""
	}
	idType := c.IDType
	if idType == "" {
		idType = channelIDType(c.Channel)
	}
	if !validIdentifier(idType, maxLabelBytes) {
		return ""
	}
	return c.Channel + ":" + idType + ":" + c.ID
}

// HeaderMapping names the HTTP headers that contain caller attributes. IDType
// is the one exception: it is a fixed identifier type, not a header name. For
// the headers adapter an omitted IDType means external_id.
type HeaderMapping struct {
	Channel string `json:"channel,omitempty"`
	ID      string `json:"id,omitempty"`
	IDType  string `json:"id_type,omitempty"`
	Name    string `json:"name,omitempty"`
	OpenID  string `json:"open_id,omitempty"`
	UnionID string `json:"union_id,omitempty"`
}

type Adapter struct {
	Kind    string        `json:"kind"`
	Headers HeaderMapping `json:"headers,omitempty"`
}

var qoderWakeHeaders = HeaderMapping{
	Channel: "X-QoderWake-AtWaker-Sender-Channel",
	ID:      "X-QoderWake-AtWaker-Sender-Id",
	Name:    "X-QoderWake-AtWaker-Sender-Name",
	OpenID:  "X-QoderWake-AtWaker-Sender-Open-Id",
	UnionID: "X-QoderWake-AtWaker-Sender-Union-Id",
}

func (a Adapter) Validate() error {
	switch a.Kind {
	case "", "none", "qoderwake":
		if a.Headers != (HeaderMapping{}) {
			return fmt.Errorf("%w: header mappings require kind=headers", ErrInvalidAdapter)
		}
		return nil
	case "headers":
		if a.Headers.Channel == "" || a.Headers.ID == "" {
			return fmt.Errorf("%w: channel and id header mappings are required", ErrInvalidAdapter)
		}
		if a.Headers.IDType != "" && !validIdentifier(a.Headers.IDType, maxLabelBytes) {
			return fmt.Errorf("%w: id_type must be a fixed identifier type", ErrInvalidAdapter)
		}
		seen := make(map[string]bool)
		for _, field := range a.Headers.fields() {
			if field.header == "" {
				continue
			}
			key := strings.ToLower(field.header)
			if !validHeaderName(field.header) || credentialHeader(key) {
				return fmt.Errorf("%w: %s must name a non-credential HTTP header", ErrInvalidAdapter, field.name)
			}
			if seen[key] {
				return fmt.Errorf("%w: header mappings must be distinct", ErrInvalidAdapter)
			}
			seen[key] = true
		}
		return nil
	default:
		return fmt.Errorf("%w: unsupported adapter kind", ErrInvalidAdapter)
	}
}

// Parse reads only the headers belonging to this adapter. All relevant header
// names must occur at most once, including differently cased map keys. Missing
// primary identifiers produce an empty Caller, while an incomplete pair is an
// error. Invalid optional display names are discarded without blocking calls.
func (a Adapter) Parse(headers http.Header) (Caller, error) {
	if err := a.Validate(); err != nil {
		return Caller{}, err
	}
	if a.Kind == "" || a.Kind == "none" {
		return Caller{}, nil
	}
	mapping := a.Headers
	if a.Kind == "qoderwake" {
		mapping = qoderWakeHeaders
	}
	values := make(map[string]string, 5)
	for _, field := range mapping.fields() {
		if field.header == "" {
			continue
		}
		value, err := singleHeader(headers, field.header)
		if err != nil {
			return Caller{}, fmt.Errorf("%w: duplicate %s header", ErrInvalidCaller, field.name)
		}
		values[field.name] = value
	}
	for _, field := range []struct {
		name string
		max  int
	}{{"channel", maxLabelBytes}, {"id", maxIDBytes}, {"open_id", maxIDBytes}, {"union_id", maxIDBytes}} {
		if value := values[field.name]; value != "" && !validIdentifier(value, field.max) {
			return Caller{}, fmt.Errorf("%w: invalid %s", ErrInvalidCaller, field.name)
		}
	}
	channel, id := values["channel"], values["id"]
	if channel == "" && id == "" {
		return Caller{}, nil
	}
	if channel == "" || id == "" {
		return Caller{}, fmt.Errorf("%w: channel and id must be supplied together", ErrInvalidCaller)
	}
	idType := mapping.IDType
	if a.Kind == "qoderwake" {
		switch channel {
		case "dingtalk", "dingtalk-user", "feishu", "lark", "wecom-bot":
			idType = channelIDType(channel)
		default:
			return Caller{}, fmt.Errorf("%w: unsupported QoderWake channel", ErrInvalidCaller)
		}
	} else if idType == "" {
		idType = "external_id"
	}
	return Caller{
		Source: a.Kind, Channel: channel, ID: id, IDType: idType,
		Name: displayName(values["name"]), OpenID: values["open_id"], UnionID: values["union_id"],
	}, nil
}

type headerField struct {
	name, header string
}

func (m HeaderMapping) fields() []headerField {
	return []headerField{{"channel", m.Channel}, {"id", m.ID}, {"name", m.Name}, {"open_id", m.OpenID}, {"union_id", m.UnionID}}
}

func singleHeader(headers http.Header, name string) (string, error) {
	var value string
	found := false
	for key, values := range headers {
		if !strings.EqualFold(key, name) {
			continue
		}
		if found || len(values) != 1 {
			return "", ErrInvalidCaller
		}
		found = true
		value = values[0]
	}
	return value, nil
}

func channelIDType(channel string) string {
	switch channel {
	case "feishu", "lark":
		return "user_id"
	case "dingtalk", "dingtalk-user":
		return "staff_id"
	case "wecom-bot":
		return "userid"
	default:
		return "external_id"
	}
}

func validIdentifier(value string, maxBytes int) bool {
	if value == "" || len(value) > maxBytes || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) || r == ':' || r == ',' {
			return false
		}
	}
	return true
}

func displayName(raw string) string {
	if len(raw) > maxRawNameBytes {
		return ""
	}
	// PathUnescape implements URI-component decoding without converting '+' to
	// a space, unlike QueryUnescape.
	name, err := url.PathUnescape(raw)
	if err != nil || len(name) > maxNameBytes || !utf8.ValidString(name) {
		return ""
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return ""
		}
	}
	return name
}

func validHeaderName(name string) bool {
	if name == "" || len(name) > maxHeaderBytes {
		return false
	}
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", r) {
			continue
		}
		return false
	}
	return true
}

func credentialHeader(name string) bool {
	return name == "cookie" || name == "set-cookie" || name == "dpop" ||
		strings.Contains(name, "authorization") || strings.Contains(name, "token") ||
		strings.Contains(name, "secret") || strings.Contains(name, "api-key") || strings.Contains(name, "apikey")
}
