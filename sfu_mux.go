package main

import "net/http"

// m7hSFURouteOverrides lets standalone serve mode replace handlers while all
// processes share one canonical SFU route registration.
type m7hSFURouteOverrides struct {
	handlers map[string]http.Handler
	metrics  http.Handler
	register func(*http.ServeMux)
}

// newM7HSFUMux registers the complete common route table, then applies handler
// overrides and additional browser routes. The route set itself stays shared.
func newM7HSFUMux(server *m7hRemoteSFUServer, token string, overrides m7hSFURouteOverrides) *http.ServeMux {
	mux := http.NewServeMux()
	register := func(path string, fallback http.Handler) {
		if h := overrides.handlers[path]; h != nil {
			mux.Handle(path, h)
		} else {
			mux.Handle(path, fallback)
		}
	}
	register("/health", server.handleHealth(token))
	register("/publisher", server.handlePublisher(token))
	register("/publisher/ready", server.handlePublisherReady(token))
	register("/viewer", server.handleViewer(token))
	register("/stats", server.handleStats(token))
	register("/receive-stats", server.handleReceiveStats(token))
	register("/heap-profile", server.handleHeapProfile(token))
	register("/goroutine-profile", server.handleGoroutineProfile(token))
	register("/cpu-profile", server.handleCPUProfile(token))
	register("/cleanup", server.handleCleanup(token))
	register("/shutdown", server.handleShutdown(token))
	metrics := overrides.metrics
	if metrics == nil {
		metrics = server.router.metricsHandler()
	}
	mux.Handle("/metrics", metrics)
	if root := overrides.handlers["/"]; root != nil {
		mux.Handle("/", root)
	} else {
		mux.Handle("/", http.NotFoundHandler())
	}
	if overrides.register != nil {
		overrides.register(mux)
	}
	return mux
}

var m7HSFURoutePaths = []string{
	"/health", "/publisher", "/publisher/ready", "/viewer", "/stats",
	"/receive-stats", "/heap-profile", "/goroutine-profile", "/cpu-profile",
	"/cleanup", "/shutdown", "/metrics",
}
