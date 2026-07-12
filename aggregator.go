package main

import (
	"context"
	"errors"
	"log"
	"sort"
	"sync"
	"time"
)

// Aggregator scrapes every provider concurrently and caches the merged result.
// The HTTP layer only ever reads the cached Snapshot, so a slow cinema site
// never blocks a page load — refreshes happen on a background schedule.
type Aggregator struct {
	providers []Provider

	mu        sync.RWMutex
	showtimes []Showtime
	errs      map[string]string // chain name -> last error (implemented chains only)
	updated   time.Time
}

func newAggregator(providers []Provider) *Aggregator {
	return &Aggregator{providers: providers, errs: map[string]string{}}
}

// Snapshot returns the current cached showtimes, per-chain errors, and the time
// of the last successful refresh cycle. Safe for concurrent readers.
func (a *Aggregator) Snapshot() ([]Showtime, map[string]string, time.Time) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	// Return copies so callers can't mutate the cache.
	st := append([]Showtime(nil), a.showtimes...)
	errs := make(map[string]string, len(a.errs))
	for k, v := range a.errs {
		errs[k] = v
	}
	return st, errs, a.updated
}

// Refresh scrapes all providers in parallel and atomically swaps in the result.
// A provider that errors keeps its previous entries out of the merge but is
// recorded in errs; a provider that's simply not implemented is ignored.
func (a *Aggregator) Refresh(ctx context.Context) {
	type result struct {
		name string
		st   []Showtime
		err  error
	}

	results := make(chan result, len(a.providers))
	var wg sync.WaitGroup
	for _, p := range a.providers {
		wg.Add(1)
		go func(p Provider) {
			defer wg.Done()
			defer func() {
				// A parser panicking on hostile markup must not take the process
				// down — treat it as that chain's error.
				if r := recover(); r != nil {
					results <- result{name: p.Name(), err: errFromRecover(r)}
				}
			}()
			st, err := p.Fetch(ctx)
			results <- result{name: p.Name(), st: st, err: err}
		}(p)
	}
	wg.Wait()
	close(results)

	var merged []Showtime
	errs := map[string]string{}
	for r := range results {
		switch {
		case errors.Is(r.err, errNotImplemented):
			// Scaffolded chain; nothing to report.
		case r.err != nil:
			errs[r.name] = r.err.Error()
			log.Printf("refresh: %s failed: %v", r.name, r.err)
		default:
			merged = append(merged, r.st...)
		}
	}
	sortShowtimes(merged)

	a.mu.Lock()
	a.showtimes = merged
	a.errs = errs
	a.updated = time.Now().In(crZone)
	a.mu.Unlock()
	log.Printf("refresh: %d showtimes from %d chains (%d errored)", len(merged), len(a.providers), len(errs))
}

// Run does an immediate refresh, then refreshes every interval until ctx is
// cancelled. Meant to be launched in its own goroutine at startup.
func (a *Aggregator) Run(ctx context.Context, interval time.Duration) {
	a.Refresh(ctx)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.Refresh(ctx)
		}
	}
}

// sortShowtimes orders by start time, then chain, cinema, movie — a stable,
// user-sensible default the frontend can re-sort from.
func sortShowtimes(s []Showtime) {
	sort.SliceStable(s, func(i, j int) bool {
		a, b := s[i], s[j]
		if !a.Start.Equal(b.Start) {
			return a.Start.Before(b.Start)
		}
		if a.Chain != b.Chain {
			return a.Chain < b.Chain
		}
		if a.Cinema != b.Cinema {
			return a.Cinema < b.Cinema
		}
		return a.Movie < b.Movie
	})
}

func errFromRecover(r any) error {
	if err, ok := r.(error); ok {
		return err
	}
	return errors.New("panic in provider")
}
