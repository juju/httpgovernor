// Copyright 2019 Canonical Ltd.

// Package httpgovernor provides concurrency limiting for a http.Handler.
package httpgovernor

import (
	"context"
	"net/http"
	"time"

	"golang.org/x/sync/semaphore"
)

type Params struct {
	// MaxConcurrency specifies the maximum level of concurrency
	// allowed by the governor. If this is 0 then concurrency will
	// not be governed.
	MaxConcurrency int64

	// MaxBurst specifies the maximum level of concurrency before
	// requests are failed without queueing. If this is 0 then no
	// requests will be queued. The maximum queue size is roughly
	// equivilent to MaxBurst-MaxConcurrency.
	MaxBurst int64

	// MaxQueueDuration specifies the maximum time a request should
	// be queued before being aborted. If this is 0 then a default
	// duration of 10s will be used.
	MaxQueueDuration time.Duration

	// OverloadHandler is the http.Handler used to handle requests
	// that have to be dropped due to the server being overloaded. If
	// this is nil then DefaultOverloadHandler will be used.
	OverloadHandler http.Handler

	// CostEstimator is used to determine the relative cost of a
	// request. If this is nil all requests will be assumed to have a
	// cost of 1.
	CostEstimator CostEstimator

	// RequestOverloadCounter is a counter that is incremented for
	// every request dropped because the server is overloaded.
	RequestOverloadCounter Counter

	// QueueLengthGauge is used to monitor the number of requests
	// queued by the governor.
	QueueLengthGauge Gauge

	// QueueDurationObserver is used to monitor the time succesful
	// requests are queued before being actioned.
	QueueDurationObserver Observer
}

// New creates a new http.Handler that wraps the given handler limiting
// the amount of concurrent requests that will be handled.
func New(p Params, hnd http.Handler) http.Handler {
	if p.MaxConcurrency == 0 {
		return hnd
	}
	if p.OverloadHandler == nil {
		p.OverloadHandler = DefaultOverloadHandler
	}
	if p.MaxBurst <= p.MaxConcurrency {
		return simpleGovernor{
			sem: semaphore.NewWeighted(p.MaxConcurrency),
			p:   p,
			hnd: hnd,
		}
	}
	if p.MaxQueueDuration == 0 {
		p.MaxQueueDuration = 10 * time.Second
	}
	return governor{
		concurrent: semaphore.NewWeighted(p.MaxConcurrency),
		burst:      semaphore.NewWeighted(p.MaxBurst),
		p:          p,
		hnd:        hnd,
	}
}

// A CostEstimator is used to determine the cost of a request.
type CostEstimator interface {
	// EstimateCost calculates the relative cost of a request, that is
	// the amount of concurrency points required to acquire before
	// servicing the request. If the cost is 0 then the request will
	// be actioned, even if there are others queued.
	EstimateCost(req *http.Request) int64
}

// A Counter is used to monitor a monotonically increasing value.
type Counter interface {
	// Inc increments the counter by 1.
	Inc()
}

// A Gauge is used to monitor the size of the request queue.
type Gauge interface {
	// Inc increments the gauge value by 1.
	Inc()
	// Dec decrements the gauge value by 1.
	Dec()
}

// An Observer is used to monitor the time taken for an action to complete.
type Observer interface {
	// Observe records the length of time (in seconds) a request had
	// to wait in the queue.
	Observe(float64)
}

// DefaultOverloadHandler is the default handler used in an overload
// condition.
var DefaultOverloadHandler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(http.StatusServiceUnavailable)
	w.Write([]byte("Overloaded"))
})

type simpleGovernor struct {
	sem *semaphore.Weighted
	p   Params
	hnd http.Handler
}

func (g simpleGovernor) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	cost := int64(1)
	if g.p.CostEstimator != nil {
		cost = g.p.CostEstimator.EstimateCost(req)
	}
	if cost == 0 {
		g.hnd.ServeHTTP(w, req)
		return
	}
	if g.sem.TryAcquire(cost) {
		defer g.sem.Release(cost)
		g.hnd.ServeHTTP(w, req)
		return
	}
	if g.p.RequestOverloadCounter != nil {
		g.p.RequestOverloadCounter.Inc()
	}
	g.p.OverloadHandler.ServeHTTP(w, req)
}

type governor struct {
	concurrent *semaphore.Weighted
	burst      *semaphore.Weighted
	p          Params
	hnd        http.Handler
}

func (g governor) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	cost := int64(1)
	if g.p.CostEstimator != nil {
		cost = g.p.CostEstimator.EstimateCost(req)
	}
	if cost == 0 {
		g.hnd.ServeHTTP(w, req)
		return
	}

	if !g.burst.TryAcquire(cost) {
		g.overload(w, req)
		return
	}
	defer g.burst.Release(cost)

	// Try to acquire the concurrent semaphore.
	if g.concurrent.TryAcquire(cost) || g.queue(req.Context(), cost) {
		defer g.concurrent.Release(cost)
		g.hnd.ServeHTTP(w, req)
		return
	}
	g.overload(w, req)
}

func (g governor) queue(ctx context.Context, cost int64) bool {
	if g.p.QueueLengthGauge != nil {
		g.p.QueueLengthGauge.Inc()
		defer g.p.QueueLengthGauge.Dec()
	}
	ctx, cancel := context.WithTimeout(ctx, g.p.MaxQueueDuration)
	defer cancel()
	start := time.Now()
	if g.concurrent.Acquire(ctx, cost) == nil {
		if g.p.QueueDurationObserver != nil {
			g.p.QueueDurationObserver.Observe(float64(time.Since(start)) / float64(time.Second))
		}
		return true
	}
	return false
}

func (g governor) overload(w http.ResponseWriter, req *http.Request) {
	if g.p.RequestOverloadCounter != nil {
		g.p.RequestOverloadCounter.Inc()
	}
	g.p.OverloadHandler.ServeHTTP(w, req)
}

// A PathCostEstimator determines the cost of a request by matching the
// path of the URL.
type PathCostEstimator map[string]int64

// EstimateCost determines the cost of the given request by matching in
// the PathCostEstimator. Any path not specified is assumed to have a
// cost of 1.
func (c PathCostEstimator) EstimateCost(req *http.Request) int64 {
	if cost, ok := c[req.URL.Path]; ok {
		return cost
	}
	return 1
}
