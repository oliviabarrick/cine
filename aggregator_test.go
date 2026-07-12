package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeProvider is a test double implementing Provider.
type fakeProvider struct {
	name string
	st   []Showtime
	err  error
}

func (f fakeProvider) Name() string                              { return f.name }
func (f fakeProvider) Fetch(context.Context) ([]Showtime, error) { return f.st, f.err }

func at(h, m int) time.Time { return time.Date(2026, 7, 12, h, m, 0, 0, crZone) }

func TestAggregatorRefreshMergesSortsAndRecordsErrors(t *testing.T) {
	agg := newAggregator([]Provider{
		fakeProvider{name: "A", st: []Showtime{
			{Chain: "A", Cinema: "A1", Movie: "Late", Start: at(21, 0), Format: FormatSubtitled},
			{Chain: "A", Cinema: "A1", Movie: "Early", Start: at(13, 0), Format: FormatDubbed},
		}},
		fakeProvider{name: "B", err: errors.New("boom")},
		fakeProvider{name: "C", err: errNotImplemented},
	})

	agg.Refresh(context.Background())
	st, errs, updated := agg.Snapshot()

	if len(st) != 2 {
		t.Fatalf("got %d showtimes, want 2 (not-implemented + errored contribute none)", len(st))
	}
	// Sorted by start time: 13:00 before 21:00.
	if st[0].Movie != "Early" || st[1].Movie != "Late" {
		t.Errorf("not sorted by start: %q then %q", st[0].Movie, st[1].Movie)
	}
	if errs["B"] != "boom" {
		t.Errorf("errors[B] = %q, want boom", errs["B"])
	}
	if _, ok := errs["C"]; ok {
		t.Error("not-implemented provider should not be recorded as an error")
	}
	if updated.IsZero() {
		t.Error("updated timestamp not set after refresh")
	}
}

func TestAggregatorRecoversFromPanic(t *testing.T) {
	agg := newAggregator([]Provider{panicProvider{}})
	agg.Refresh(context.Background()) // must not panic the test process
	_, errs, _ := agg.Snapshot()
	if _, ok := errs["panicky"]; !ok {
		t.Error("panicking provider should be recorded as an error")
	}
}

type panicProvider struct{}

func (panicProvider) Name() string                              { return "panicky" }
func (panicProvider) Fetch(context.Context) ([]Showtime, error) { panic("bad markup") }
