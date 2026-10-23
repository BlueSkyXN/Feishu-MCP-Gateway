// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"context"
	"io"
	"mime"
	"net/http"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type privateWriter struct{ http.ResponseWriter }

func (w privateWriter) WriteHeader(code int) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.ResponseWriter.WriteHeader(code)
}
func (w privateWriter) Write(p []byte) (int, error) {
	w.Header().Set("Cache-Control", "private, no-store")
	return w.ResponseWriter.Write(p)
}
func (w privateWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w privateWriter) Flush() {
	w.Header().Set("Cache-Control", "private, no-store")
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

func (s *Service) Handler() http.Handler {
	transport := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		p, _ := principal(r.Context())
		return s.policy.Load().views[p.ClientID]
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	mux := http.NewServeMux()
	mux.Handle("/mcp", transport)
	mux.HandleFunc("POST /files", s.upload)
	mux.HandleFunc("GET /files/{id}", s.download)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		if !s.beginRequest() {
			http.Error(w, "service stopping", 503)
			return
		}
		defer s.calls.Done()
		w = privateWriter{w}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if origin := r.Header.Get("Origin"); origin != "" && !slices.Contains(s.currentConfig().Origins, origin) {
			http.Error(w, "Origin is not allowed", http.StatusForbidden)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/events/") {
			s.eventHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/transfers/") {
			s.transferHTTP(w, r)
			return
		}
		if r.URL.Path == "/admin/status" || r.URL.Path == "/admin/reload" {
			s.admin(w, r)
			return
		}
		p, err := s.currentConfig().authenticate(r)
		if err != nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="feishu-mcp-gateway"`)
			code := http.StatusUnauthorized
			if f, ok := err.(*Fault); ok && f.Kind != "unauthenticated" {
				code = http.StatusForbidden
			}
			http.Error(w, err.Error(), code)
			return
		}
		max := int64(2 << 20)
		if r.URL.Path == "/files" {
			max = s.config.MaxFileBytes
		}
		r.Body = http.MaxBytesReader(w, r.Body, max)
		mux.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, p)))
	})
}
func (s *Service) upload(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if _, _, err := s.currentConfig().checkPrincipal(p, true); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if err := s.rate(p); err != nil {
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}
	if err := s.acquirePrincipal(p); err != nil {
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}
	defer s.releasePrincipal(p)
	b, err := readBounded(r.Body, s.config.MaxFileBytes)
	if err != nil {
		http.Error(w, "file exceeds size limit or cannot be read", http.StatusRequestEntityTooLarge)
		return
	}
	typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		typ = "application/octet-stream"
	}
	f, err := s.Store.PutFile(p, r.URL.Query().Get("name"), typ, b)
	if err != nil {
		http.Error(w, asFault(err).Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(encode(f))
}
func (s *Service) download(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if _, _, err := s.currentConfig().checkPrincipal(p, true); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if err := s.rate(p); err != nil {
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}
	if err := s.acquirePrincipal(p); err != nil {
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}
	defer s.releasePrincipal(p)
	f, err := s.AuthorizeFile(p, r.PathValue("id"))
	if err != nil {
		http.Error(w, asFault(err).Error(), http.StatusNotFound)
		return
	}
	in, err := s.Store.files.Open(f.ID)
	if err != nil {
		http.Error(w, "file unavailable", http.StatusNotFound)
		return
	}
	defer in.Close()
	w.Header().Set("Content-Type", f.MIME)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": f.Name}))
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = io.Copy(w, io.LimitReader(in, f.Size))
}
