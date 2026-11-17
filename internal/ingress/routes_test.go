// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package ingress

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func testCaller() Caller {
	return Caller{Source: "qoderwake", Channel: "feishu", ID: "alice", IDType: "user_id", Name: "Alice", OpenID: "ou_alice", UnionID: "on_alice"}
}

func TestRouteMatchesAllSpecifiedFields(t *testing.T) {
	route := Route{ID: "personal", Priority: 10, AppConnectionID: "app-a", Match: Match{
		Sources: []string{"headers", "qoderwake"}, Channels: []string{"lark", "feishu"}, IDs: []string{"bob", "alice"},
		IDTypes: []string{"user_id"}, OpenIDs: []string{"ou_alice"}, UnionIDs: []string{"on_alice"},
	}}
	for _, tc := range []struct {
		name   string
		change func(*Caller)
		match  bool
	}{
		{"all attributes", func(*Caller) {}, true},
		{"name is irrelevant", func(c *Caller) { c.Name = "Bob" }, true},
		{"another OR value", func(c *Caller) { c.Channel = "lark" }, true},
		{"source", func(c *Caller) { c.Source = "other" }, false},
		{"channel", func(c *Caller) { c.Channel = "dingtalk" }, false},
		{"case sensitive", func(c *Caller) { c.ID = "Alice" }, false},
		{"id type", func(c *Caller) { c.IDType = "staff_id" }, false},
		{"open id", func(c *Caller) { c.OpenID = "ou_bob" }, false},
		{"union id", func(c *Caller) { c.UnionID = "on_bob" }, false},
		{"missing optional match", func(c *Caller) { c.UnionID = "" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := testCaller()
			tc.change(&caller)
			got, err := Select([]Route{route}, caller, "", "default")
			want := Selection{AppConnectionID: "default"}
			if tc.match {
				want = Selection{RouteID: "personal", AppConnectionID: "app-a"}
			}
			if err != nil || got != want {
				t.Fatalf("Select() = %+v, %v; want %+v", got, err, want)
			}
		})
	}
}

func TestHighestPriorityIndependentOfOrder(t *testing.T) {
	routes := []Route{
		{ID: "low-a", Priority: -10, AppConnectionID: "app-low"},
		{ID: "low-b", Priority: -10, AppConnectionID: "app-low"},
		{ID: "winner", Priority: -1, AppConnectionID: "app-winner", Match: Match{IDs: []string{"alice"}}},
		{ID: "unmatched", Priority: 100, AppConnectionID: "app-other", Match: Match{IDs: []string{"bob"}}},
	}
	for range 2 {
		before := slices.Clone(routes)
		got, err := Select(routes, testCaller(), "", "default")
		if err != nil || got != (Selection{RouteID: "winner", AppConnectionID: "app-winner"}) {
			t.Fatalf("priority selection = %+v, %v", got, err)
		}
		if !reflect.DeepEqual(before, routes) {
			t.Fatal("Select changed route order or contents")
		}
		slices.Reverse(routes)
	}
}

func TestHighestPriorityTieIsAmbiguous(t *testing.T) {
	for _, sameApp := range []bool{false, true} {
		app := "app-b"
		if sameApp {
			app = "app-a"
		}
		routes := []Route{{ID: "a", Priority: 5, AppConnectionID: "app-a"}, {ID: "b", Priority: 5, AppConnectionID: app}}
		got, err := Select(routes, testCaller(), "", "default")
		if !errors.Is(err, ErrRouteAmbiguous) || got != (Selection{}) {
			t.Fatalf("equal priorities were silently selected: %+v, %v", got, err)
		}
	}
}

func TestExplicitRouteNeverFallsBack(t *testing.T) {
	routes := []Route{
		{ID: "default-route", Priority: 100, AppConnectionID: "default"},
		{ID: "alice", Priority: 1, AppConnectionID: "app-a", Match: Match{IDs: []string{"alice"}}},
		{ID: "bob", Priority: 1, AppConnectionID: "app-b", Match: Match{IDs: []string{"bob"}}},
	}
	got, err := Select(routes, testCaller(), "alice", "fallback")
	if err != nil || got != (Selection{RouteID: "alice", AppConnectionID: "app-a"}) {
		t.Fatalf("explicit lower priority route not honored: %+v, %v", got, err)
	}
	for _, tc := range []struct {
		requested string
		want      error
	}{{"missing", ErrRouteNotFound}, {"bob", ErrRouteDenied}} {
		got, err := Select(routes, testCaller(), tc.requested, "fallback")
		if !errors.Is(err, tc.want) || got != (Selection{}) {
			t.Fatalf("requested %q = %+v, %v; want %v", tc.requested, got, err, tc.want)
		}
	}
}

func TestAbsentIdentityCannotMatchConstrainedRoute(t *testing.T) {
	for _, tc := range []struct {
		caller Caller
		match  Match
	}{
		{Caller{}, Match{IDs: []string{"alice"}}},
		{Caller{Source: "qoderwake"}, Match{Sources: []string{"qoderwake"}}},
		{Caller{Channel: "feishu"}, Match{Channels: []string{"feishu"}}},
		{Caller{ID: "alice"}, Match{IDs: []string{"alice"}}},
		{Caller{OpenID: "ou_alice"}, Match{OpenIDs: []string{"ou_alice"}}},
		{Caller{UnionID: "on_alice"}, Match{UnionIDs: []string{"on_alice"}}},
	} {
		routes := []Route{{ID: "restricted", AppConnectionID: "app-a", Match: tc.match}}
		got, err := Select(routes, tc.caller, "", "default")
		if err != nil || got != (Selection{AppConnectionID: "default"}) {
			t.Fatalf("incomplete caller matched constrained route: %+v, %v", got, err)
		}
		if _, err = Select(routes, tc.caller, "restricted", "default"); !errors.Is(err, ErrRouteDenied) {
			t.Fatalf("explicit constrained route accepted incomplete caller: %v", err)
		}
	}
	got, err := Select([]Route{{ID: "catch-all", AppConnectionID: "app-a"}}, Caller{}, "", "default")
	if err != nil || got != (Selection{RouteID: "catch-all", AppConnectionID: "app-a"}) {
		t.Fatalf("empty caller failed catch-all route: %+v, %v", got, err)
	}
}

func TestDefaultWithNoMatchingRoute(t *testing.T) {
	for _, routes := range [][]Route{nil, {{ID: "other", AppConnectionID: "app-b", Match: Match{IDs: []string{"bob"}}}}} {
		for _, defaultApp := range []string{"default", ""} {
			got, err := Select(routes, testCaller(), "", defaultApp)
			if err != nil || got != (Selection{AppConnectionID: defaultApp}) {
				t.Fatalf("default selection = %+v, %v", got, err)
			}
		}
	}
}

func TestValidateRoutes(t *testing.T) {
	if err := ValidateRoutes(nil); err != nil {
		t.Fatalf("empty routes rejected: %v", err)
	}
	for _, routes := range [][]Route{
		{{AppConnectionID: "app-a"}}, {{ID: "a"}},
		{{ID: "a", AppConnectionID: "app-a"}, {ID: "a", AppConnectionID: "app-b"}},
		{{ID: "bad route", AppConnectionID: "app-a"}},
		{{ID: strings.Repeat("r", 129), AppConnectionID: "app-a"}},
		{{ID: "用户-route", AppConnectionID: "app-a"}},
		{{ID: "a", AppConnectionID: "bad\napp"}},
		{{ID: "a", AppConnectionID: "app-a", Match: Match{IDs: []string{""}}}},
		{{ID: "a", AppConnectionID: "app-a", Match: Match{IDs: []string{"alice", "alice"}}}},
		{{ID: "a", AppConnectionID: "app-a", Match: Match{Channels: []string{"feishu:invalid"}}}},
	} {
		if err := ValidateRoutes(routes); !errors.Is(err, ErrInvalidRoute) {
			t.Fatalf("invalid route accepted: %+v: %v", routes, err)
		}
		if _, err := Select(routes, testCaller(), "", "default"); !errors.Is(err, ErrInvalidRoute) {
			t.Fatalf("Select bypassed route validation: %+v: %v", routes, err)
		}
	}
	if err := ValidateRoutes([]Route{{ID: strings.Repeat("r", 128), AppConnectionID: "应用:连接"}}); err != nil {
		t.Fatalf("header-compatible route boundary or unchanged application reference rejected: %v", err)
	}
}
