// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
// HTU normalization adapted from the pinned lark-cli source.
package gateway

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// NormalizeHTU produces the RFC 3986 form used by DPoP htu: scheme and host
// are lowercase, default ports/query/fragment/dot segments are removed, and
// percent encodings are canonicalized without changing path case.
func NormalizeHTU(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return "", fmt.Errorf("invalid DPoP request URL")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("invalid DPoP request URL scheme %q", parsed.Scheme)
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return "", errors.New("invalid DPoP request URL host")
	}
	port := parsed.Port()
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		port = ""
	}
	authority := host
	if strings.Contains(host, ":") {
		authority = "[" + host + "]"
	}
	if port != "" {
		authority = net.JoinHostPort(host, port)
	}
	escapedPath := parsed.EscapedPath()
	if escapedPath == "" {
		escapedPath = "/"
	}
	escapedPath, err = normalizePercentEncoding(escapedPath)
	if err != nil {
		return "", fmt.Errorf("invalid DPoP request URL path: %w", err)
	}
	escapedPath = removeDotSegments(escapedPath)
	if escapedPath == "" {
		escapedPath = "/"
	}
	return scheme + "://" + authority + escapedPath, nil
}

func normalizePercentEncoding(value string) (string, error) {
	var out strings.Builder
	out.Grow(len(value))
	for i := 0; i < len(value); i++ {
		if value[i] != '%' {
			out.WriteByte(value[i])
			continue
		}
		if i+2 >= len(value) {
			return "", errors.New("truncated percent encoding")
		}
		high, okHigh := hexValue(value[i+1])
		low, okLow := hexValue(value[i+2])
		if !okHigh || !okLow {
			return "", errors.New("invalid percent encoding")
		}
		decoded := high<<4 | low
		if isUnreserved(decoded) {
			out.WriteByte(decoded)
		} else {
			const upperHex = "0123456789ABCDEF"
			out.WriteByte('%')
			out.WriteByte(upperHex[decoded>>4])
			out.WriteByte(upperHex[decoded&0x0f])
		}
		i += 2
	}
	return out.String(), nil
}

func hexValue(value byte) (byte, bool) {
	switch {
	case value >= '0' && value <= '9':
		return value - '0', true
	case value >= 'a' && value <= 'f':
		return value - 'a' + 10, true
	case value >= 'A' && value <= 'F':
		return value - 'A' + 10, true
	default:
		return 0, false
	}
}

func isUnreserved(value byte) bool {
	return value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9' ||
		value == '-' || value == '.' || value == '_' || value == '~'
}

// removeDotSegments implements RFC 3986 section 5.2.4 without collapsing
// ordinary empty path segments.
func removeDotSegments(input string) string {
	var output string
	for input != "" {
		switch {
		case strings.HasPrefix(input, "../"):
			input = input[3:]
		case strings.HasPrefix(input, "./"):
			input = input[2:]
		case strings.HasPrefix(input, "/./"):
			input = "/" + input[3:]
		case input == "/.":
			input = "/"
		case strings.HasPrefix(input, "/../"):
			input = "/" + input[4:]
			output = removeLastPathSegment(output)
		case input == "/..":
			input = "/"
			output = removeLastPathSegment(output)
		case input == "." || input == "..":
			input = ""
		default:
			next := len(input)
			start := 0
			if input[0] == '/' {
				start = 1
			}
			if index := strings.IndexByte(input[start:], '/'); index >= 0 {
				next = start + index
			}
			output += input[:next]
			input = input[next:]
		}
	}
	return output
}

func removeLastPathSegment(path string) string {
	if index := strings.LastIndexByte(path, '/'); index >= 0 {
		return path[:index]
	}
	return ""
}
