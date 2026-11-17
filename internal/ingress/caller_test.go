// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package ingress

import (
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func wakeHeaders(channel, id string) http.Header {
	h := make(http.Header)
	if channel != "" {
		h.Set(qoderWakeHeaders.Channel, channel)
	}
	if id != "" {
		h.Set(qoderWakeHeaders.ID, id)
	}
	return h
}

func TestQoderWakeCaller(t *testing.T) {
	h := wakeHeaders("feishu", "alice")
	h.Set(qoderWakeHeaders.Name, "%E7%94%A8%E6%88%B7+A%2BB")
	h.Set(qoderWakeHeaders.OpenID, "ou_alice")
	h.Set(qoderWakeHeaders.UnionID, "on_alice")
	got, err := (Adapter{Kind: "qoderwake"}).Parse(h)
	want := Caller{Source: "qoderwake", Channel: "feishu", ID: "alice", IDType: "user_id", Name: "用户+A+B", OpenID: "ou_alice", UnionID: "on_alice"}
	if err != nil || got != want {
		t.Fatalf("Parse() = %+v, %v; want %+v", got, err, want)
	}
	if got.ActorKey() != "feishu:user_id:alice" {
		t.Fatalf("ActorKey() = %q", got.ActorKey())
	}
	got.Name, got.OpenID, got.UnionID = "another name", "ou_other", "on_other"
	if got.ActorKey() != "feishu:user_id:alice" {
		t.Fatal("supplemental metadata changed the primary actor key")
	}
}

func TestQoderWakeChannels(t *testing.T) {
	for _, tc := range []struct{ channel, idType string }{
		{"feishu", "user_id"}, {"lark", "user_id"}, {"dingtalk", "staff_id"},
		{"dingtalk-user", "staff_id"}, {"wecom-bot", "userid"},
	} {
		t.Run(tc.channel, func(t *testing.T) {
			got, err := (Adapter{Kind: "qoderwake"}).Parse(wakeHeaders(tc.channel, "alice"))
			if err != nil || got.Source != "qoderwake" || got.IDType != tc.idType || got.ActorKey() != tc.channel+":"+tc.idType+":alice" {
				t.Fatalf("Parse() = %+v, %v", got, err)
			}
		})
	}
}

func TestOptionalCaller(t *testing.T) {
	for _, h := range []http.Header{nil, {}, {qoderWakeHeaders.ID: {""}, qoderWakeHeaders.Channel: {""}}, {qoderWakeHeaders.Name: {"Alice"}, qoderWakeHeaders.OpenID: {"ou_alice"}}} {
		got, err := (Adapter{Kind: "qoderwake"}).Parse(h)
		if err != nil || got != (Caller{}) {
			t.Fatalf("missing primary identity = %+v, %v", got, err)
		}
	}
	for _, kind := range []string{"", "none"} {
		got, err := (Adapter{Kind: kind}).Parse(wakeHeaders("feishu", "alice"))
		if err != nil || got != (Caller{}) {
			t.Fatalf("disabled adapter accepted caller metadata: %+v, %v", got, err)
		}
	}
}

func TestRejectInvalidCaller(t *testing.T) {
	tests := []struct {
		name string
		edit func(http.Header)
	}{
		{"missing id", func(h http.Header) { h.Del(qoderWakeHeaders.ID) }},
		{"missing channel", func(h http.Header) { h.Del(qoderWakeHeaders.Channel) }},
		{"unknown channel", func(h http.Header) { h.Set(qoderWakeHeaders.Channel, "slack") }},
		{"noncanonical channel", func(h http.Header) { h.Set(qoderWakeHeaders.Channel, "Feishu") }},
		{"colon in id", func(h http.Header) { h.Set(qoderWakeHeaders.ID, "user:alice") }},
		{"joined ids", func(h http.Header) { h.Set(qoderWakeHeaders.ID, "alice,bob") }},
		{"space in id", func(h http.Header) { h.Set(qoderWakeHeaders.ID, " alice") }},
		{"control in id", func(h http.Header) { h.Set(qoderWakeHeaders.ID, "alice\x00") }},
		{"newline in channel", func(h http.Header) { h.Set(qoderWakeHeaders.Channel, "feishu\r\n") }},
		{"invalid utf8", func(h http.Header) { h.Set(qoderWakeHeaders.ID, "\xff") }},
		{"long id", func(h http.Header) { h.Set(qoderWakeHeaders.ID, strings.Repeat("x", maxIDBytes+1)) }},
		{"long channel", func(h http.Header) { h.Set(qoderWakeHeaders.Channel, strings.Repeat("x", maxLabelBytes+1)) }},
		{"long open id", func(h http.Header) { h.Set(qoderWakeHeaders.OpenID, strings.Repeat("x", maxIDBytes+1)) }},
		{"control in union id", func(h http.Header) { h.Set(qoderWakeHeaders.UnionID, "on_\tbad") }},
		{"multiple id values", func(h http.Header) { h.Add(qoderWakeHeaders.ID, "alice") }},
		{"case variant id", func(h http.Header) { h[strings.ToLower(qoderWakeHeaders.ID)] = []string{"alice"} }},
		{"case variant channel", func(h http.Header) { h[strings.ToLower(qoderWakeHeaders.Channel)] = []string{"feishu"} }},
		{"multiple name values", func(h http.Header) { h[qoderWakeHeaders.Name] = []string{"Alice", "Bob"} }},
		{"multiple open id values", func(h http.Header) { h[qoderWakeHeaders.OpenID] = []string{"ou_a", "ou_b"} }},
		{"multiple union id values", func(h http.Header) { h[qoderWakeHeaders.UnionID] = []string{"on_a", "on_b"} }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := wakeHeaders("feishu", "alice")
			tc.edit(h)
			got, err := (Adapter{Kind: "qoderwake"}).Parse(h)
			if !errors.Is(err, ErrInvalidCaller) || got != (Caller{}) {
				t.Fatalf("invalid metadata = %+v, %v", got, err)
			}
		})
	}
}

func TestMalformedDisplayNameDoesNotBlockCaller(t *testing.T) {
	for _, name := range []string{"bad%escape", "%0Anew-line", "%00", "%FF", strings.Repeat("x", maxNameBytes+1), strings.Repeat("x", maxRawNameBytes+1)} {
		h := wakeHeaders("feishu", "alice")
		h.Set(qoderWakeHeaders.Name, name)
		got, err := (Adapter{Kind: "qoderwake"}).Parse(h)
		if err != nil || got.Name != "" || got.ActorKey() != "feishu:user_id:alice" {
			t.Fatalf("malformed name blocked or changed identity: %+v, %v", got, err)
		}
	}
}

func TestCustomHeaderAdapter(t *testing.T) {
	adapter := Adapter{Kind: "headers", Headers: HeaderMapping{
		Channel: "X-Caller-Channel", ID: "X-Caller-Id", Name: "X-Caller-Name",
		OpenID: "X-Caller-Open-Id", UnionID: "X-Caller-Union-Id",
	}}
	h := http.Header{
		"x-caller-channel": {"slack"}, "x-caller-id": {"U123"}, "x-caller-name": {"A%20B+C"},
		"x-caller-open-id": {"open-1"}, "x-caller-union-id": {"union-1"},
		"Authorization": {"Bearer ignored"},
	}
	got, err := adapter.Parse(h)
	want := Caller{Source: "headers", Channel: "slack", ID: "U123", IDType: "external_id", Name: "A B+C", OpenID: "open-1", UnionID: "union-1"}
	if err != nil || got != want {
		t.Fatalf("custom Parse() = %+v, %v; want %+v", got, err, want)
	}
	adapter.Headers.IDType = "member_id"
	got, err = adapter.Parse(h)
	if err != nil || got.ActorKey() != "slack:member_id:U123" {
		t.Fatalf("fixed IDType = %+v, %v", got, err)
	}
	h["X-CALLER-ID"] = []string{"U123"}
	if _, err = adapter.Parse(h); !errors.Is(err, ErrInvalidCaller) {
		t.Fatalf("case variant duplicate accepted: %v", err)
	}
}

func TestAdapterValidation(t *testing.T) {
	valid := []Adapter{{}, {Kind: "none"}, {Kind: "qoderwake"}, {Kind: "headers", Headers: HeaderMapping{Channel: "X-Channel", ID: "X-ID", IDType: "member_id"}}}
	for _, a := range valid {
		if err := a.Validate(); err != nil {
			t.Fatalf("valid adapter rejected: %+v: %v", a, err)
		}
	}
	invalid := []Adapter{
		{Kind: "unknown"}, {Headers: HeaderMapping{ID: "X-ID"}},
		{Kind: "qoderwake", Headers: HeaderMapping{ID: "X-ID"}},
		{Kind: "headers"}, {Kind: "headers", Headers: HeaderMapping{ID: "X-ID"}},
		{Kind: "headers", Headers: HeaderMapping{Channel: "X-Channel"}},
		{Kind: "headers", Headers: HeaderMapping{Channel: "X-Same", ID: "x-same"}},
		{Kind: "headers", Headers: HeaderMapping{Channel: "X-Channel", ID: "X-ID", IDType: "bad:type"}},
	}
	for _, header := range []string{"Authorization", "proxy-AUTHORIZATION", "X-Feishu-Access-Token", "X-Feishu-Upstream-Token", "X-App-Secret", "X-API-Key", "DPoP", "Cookie", "Set-Cookie", "Bad:Name", "Bad Name", "X-\r\nID", "X-中文", strings.Repeat("x", maxHeaderBytes+1)} {
		invalid = append(invalid, Adapter{Kind: "headers", Headers: HeaderMapping{Channel: "X-Channel", ID: header}})
	}
	for _, a := range invalid {
		if err := a.Validate(); !errors.Is(err, ErrInvalidAdapter) {
			t.Fatalf("invalid adapter accepted: %+v: %v", a, err)
		}
		if _, err := a.Parse(nil); !errors.Is(err, ErrInvalidAdapter) {
			t.Fatalf("Parse bypassed adapter validation: %+v: %v", a, err)
		}
	}
}

func TestActorKeyDefaultsAndEmptyIdentity(t *testing.T) {
	for _, tc := range []struct {
		caller Caller
		want   string
	}{
		{Caller{}, ""}, {Caller{Name: "Alice", OpenID: "ou_a"}, ""},
		{Caller{ID: "alice"}, ""}, {Caller{Channel: "feishu"}, ""},
		{Caller{Channel: "feishu", ID: "alice"}, "feishu:user_id:alice"},
		{Caller{Channel: "lark", ID: "alice"}, "lark:user_id:alice"},
		{Caller{Channel: "dingtalk", ID: "alice"}, "dingtalk:staff_id:alice"},
		{Caller{Channel: "dingtalk-user", ID: "alice"}, "dingtalk-user:staff_id:alice"},
		{Caller{Channel: "wecom-bot", ID: "alice"}, "wecom-bot:userid:alice"},
		{Caller{Channel: "custom", ID: "alice"}, "custom:external_id:alice"},
		{Caller{Channel: "custom", ID: "alice", IDType: "member"}, "custom:member:alice"},
		{Caller{Channel: "feishu", ID: "bad:id"}, ""},
		{Caller{Channel: "feishu", ID: "alice", IDType: "bad:type"}, ""},
	} {
		before := tc.caller
		if got := tc.caller.ActorKey(); got != tc.want {
			t.Errorf("ActorKey(%+v) = %q; want %q", tc.caller, got, tc.want)
		}
		if !reflect.DeepEqual(before, tc.caller) {
			t.Fatal("ActorKey mutated caller")
		}
	}
}
