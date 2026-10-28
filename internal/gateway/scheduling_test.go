// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"context"
	"testing"
	"time"
)

func TestQueuedJobsSkipBusyPrincipals(t *testing.T) {
	for _, scope := range []string{"client", "app", "actor"} {
		t.Run(scope, func(t *testing.T) {
			c := testConfig(t)
			switch scope {
			case "client":
				c.Clients[0].MaxConcurrent = 1
			case "app":
				c.Apps[0].MaxConcurrent = 1
			case "actor":
				policy := c.Clients[0].Actors[actor("alice").Actor]
				policy.MaxConcurrent = 1
				c.Clients[0].Actors[actor("alice").Actor] = policy
			}
			started := make(chan struct{}, 8)
			other := make(chan struct{}, 1)
			f := &fakeExecutor{fn: func(ctx context.Context, in Invocation, _ App) ExecutionResult {
				if in.Principal.ClientID == "wake" {
					started <- struct{}{}
					<-ctx.Done()
					return ExecutionResult{Error: fault("cancelled", "fixture stopped")}
				}
				other <- struct{}{}
				return ExecutionResult{Data: map[string]any{}}
			}}
			s := startService(t, c, f)
			jobs := []string{}
			for i := 0; i < c.Workers; i++ {
				args := argsSpace("space-a")
				args["async"] = true
				r := s.Invoke(context.Background(), actor("alice"), "wiki.spaces.get", args)
				if r.Error != nil {
					t.Fatal(r.Error)
				}
				jobs = append(jobs, r.InvocationID)
			}
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("first job did not start")
			}
			args := argsSpace("space-b")
			args["async"] = true
			p := Principal{ClientID: "desktop", AppID: "app-b", OrgID: "org-b"}
			if r := s.Invoke(context.Background(), p, "wiki.spaces.get", args); r.Error != nil {
				t.Fatal(r.Error)
			}
			select {
			case <-other:
			case <-time.After(3 * time.Second):
				t.Fatal("busy principal blocked an independent client")
			}
			select {
			case <-started:
				t.Fatal("principal concurrency limit exceeded")
			default:
			}
			pending := []string{}
			for _, id := range jobs {
				r, err := s.Store.Record(actor("alice"), id)
				if err != nil {
					t.Fatal(err)
				}
				if r.State == "queued" {
					pending = append(pending, id)
				}
			}
			if len(pending) != c.Workers-1 {
				t.Fatalf("waiting jobs left queued state: %v", pending)
			}
			if r := s.Job(actor("alice"), pending[0], true); r.Status != "cancelled" {
				t.Fatalf("queued cancellation: %+v", r)
			}
			c.Clients[0].Disabled = true
			if err := s.Reload(c); err != nil {
				t.Fatal(err)
			}
			for _, id := range pending[1:] {
				deadline := time.Now().Add(3 * time.Second)
				for {
					r, err := s.Store.Record(actor("alice"), id)
					if err != nil {
						t.Fatal(err)
					}
					if r.State == "failed" {
						if r.Result.Error == nil || r.Result.Error.Kind != "access_revoked" {
							t.Fatalf("revoked job: %+v", r)
						}
						break
					}
					if time.Now().After(deadline) {
						t.Fatalf("revoked queued job stuck: %+v", r)
					}
					time.Sleep(10 * time.Millisecond)
				}
			}
		})
	}
}
