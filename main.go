// Command cine serves a Costa Rica movie-showtimes aggregator: it scrapes the
// local cinema chains (Cinépolis, Cinemark, CCM, Sala Garbo), normalizes their
// listings into a single structured feed, and serves a filterable browser UI —
// filter by movie, time, subtitled/dubbed, and location.
//
// Scraping runs server-side on a background schedule into an in-memory cache, so
// page loads only ever read cached data. Structure and the static-asset/CI/deploy
// pipeline mirror the photo-editor template this repo is based on.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	webDir          = "web"
	refreshInterval = 30 * time.Minute
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	agg := newAggregator(providers())
	go agg.Run(ctx, refreshInterval)

	srv, err := newServer(webDir, agg)
	if err != nil {
		log.Fatalf("cannot build server from %q: %v", webDir, err)
	}

	httpSrv := &http.Server{Addr: ":8080", Handler: gzipMiddleware(srv)}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()

	log.Printf("cine listening on :8080 (bundle version %s, refresh every %s)", srv.version, refreshInterval)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
	os.Exit(0)
}
