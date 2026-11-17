// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package ingress

import (
	"errors"
	"fmt"
	"slices"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalidRoute   = errors.New("invalid application route")
	ErrRouteNotFound  = errors.New("application route not found")
	ErrRouteAmbiguous = errors.New("ambiguous application route")
	ErrRouteDenied    = errors.New("caller does not match application route")
)

type Route struct {
	ID              string `json:"id"`
	Priority        int    `json:"priority,omitempty"`
	Match           Match  `json:"match,omitempty"`
	AppConnectionID string `json:"app_connection_id"`
}

// Match uses exact, case-sensitive comparisons. Nonempty fields are combined
// with AND; values within each field are combined with OR. An empty Match is a
// catch-all rule. Display names cannot be used for routing.
type Match struct {
	Sources  []string `json:"sources,omitempty"`
	Channels []string `json:"channels,omitempty"`
	IDs      []string `json:"ids,omitempty"`
	IDTypes  []string `json:"id_types,omitempty"`
	OpenIDs  []string `json:"open_ids,omitempty"`
	UnionIDs []string `json:"union_ids,omitempty"`
}

type Selection struct {
	RouteID         string `json:"route_id,omitempty"`
	AppConnectionID string `json:"app_connection_id"`
}

func ValidateRoutes(routes []Route) error {
	seen := make(map[string]bool, len(routes))
	for _, route := range routes {
		if !validRouteID(route.ID) || !validReference(route.AppConnectionID) {
			return fmt.Errorf("%w: each route needs a valid id and app_connection_id", ErrInvalidRoute)
		}
		if seen[route.ID] {
			return fmt.Errorf("%w: route ids must be unique", ErrInvalidRoute)
		}
		seen[route.ID] = true
		for _, field := range route.Match.fields(Caller{}) {
			values := make(map[string]bool, len(field.allowed))
			for _, value := range field.allowed {
				if !validIdentifier(value, field.maxBytes) {
					return fmt.Errorf("%w: invalid %s match value", ErrInvalidRoute, field.name)
				}
				if values[value] {
					return fmt.Errorf("%w: duplicate %s match value", ErrInvalidRoute, field.name)
				}
				values[value] = true
			}
		}
	}
	return nil
}

// Select honors an explicit route ID only if that route matches the caller.
// Without an explicit ID, only the highest-priority matching rules participate
// in selection; a tie is an error, even when the tied rules target the same app.
// A missing match returns the configured default without inventing a route ID.
func Select(routes []Route, caller Caller, requestedID, defaultApp string) (Selection, error) {
	if err := ValidateRoutes(routes); err != nil {
		return Selection{}, err
	}
	if requestedID != "" {
		for _, route := range routes {
			if route.ID != requestedID {
				continue
			}
			if !route.Match.matches(caller) {
				return Selection{}, ErrRouteDenied
			}
			return selection(route), nil
		}
		return Selection{}, ErrRouteNotFound
	}
	var chosen Route
	found, ambiguous := false, false
	for _, route := range routes {
		if !route.Match.matches(caller) {
			continue
		}
		if !found || route.Priority > chosen.Priority {
			chosen, found, ambiguous = route, true, false
		} else if route.Priority == chosen.Priority {
			ambiguous = true
		}
	}
	if ambiguous {
		return Selection{}, ErrRouteAmbiguous
	}
	if found {
		return selection(chosen), nil
	}
	return Selection{AppConnectionID: defaultApp}, nil
}

func selection(route Route) Selection {
	return Selection{RouteID: route.ID, AppConnectionID: route.AppConnectionID}
}

type matchField struct {
	name     string
	allowed  []string
	actual   string
	maxBytes int
}

func (m Match) fields(c Caller) []matchField {
	return []matchField{
		{"sources", m.Sources, c.Source, maxLabelBytes},
		{"channels", m.Channels, c.Channel, maxLabelBytes},
		{"ids", m.IDs, c.ID, maxIDBytes},
		{"id_types", m.IDTypes, c.IDType, maxLabelBytes},
		{"open_ids", m.OpenIDs, c.OpenID, maxIDBytes},
		{"union_ids", m.UnionIDs, c.UnionID, maxIDBytes},
	}
}

func (m Match) matches(c Caller) bool {
	for _, field := range m.fields(c) {
		if len(field.allowed) == 0 {
			continue
		}
		// A partial or absent primary identity must not satisfy a constrained
		// rule using only supplemental fields such as source or open_id.
		if c.ActorKey() == "" || !slices.Contains(field.allowed, field.actual) {
			return false
		}
	}
	return true
}

func validReference(value string) bool {
	if value == "" || len(value) > 256 || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

// Route IDs are also accepted in X-Feishu-Gateway-Route, so configuration and
// explicit request selection share the same 128-byte printable-ASCII format.
func validRouteID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, ch := range value {
		if ch < 0x21 || ch > 0x7e {
			return false
		}
	}
	return true
}
