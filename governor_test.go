// Copyright 2019 Canonical Ltd.

package httpgovernor_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"

	"github.com/juju/httpgovernor"
)

func TestNoGovernor(t *testing.T) {
	c := qt.New(t)

	req := httptest.NewRequest("", "/", nil)
	startc := make(chan struct{})
	req = req.WithContext(context.WithValue(req.Context(), testHandlerStartKey{}, startc))
	finishc := make(chan struct{})
	req = req.WithContext(context.WithValue(req.Context(), testHandlerFinishKey{}, finishc))

	var success, overload uint32

	hnd := httpgovernor.New(httpgovernor.Params{}, testHandler)
	var wg sync.WaitGroup
	wg.Add(3)
	go doReq(wg.Done, hnd, req, &success, &overload)
	go doReq(wg.Done, hnd, req, &success, &overload)
	go doReq(wg.Done, hnd, req, &success, &overload)
	<-startc
	<-startc
	<-startc
	close(finishc)
	wg.Wait()

	c.Assert(atomic.LoadUint32(&success), qt.Equals, uint32(3))
	c.Assert(atomic.LoadUint32(&overload), qt.Equals, uint32(0))
}

func TestSimpleGovernor(t *testing.T) {
	c := qt.New(t)

	req := httptest.NewRequest("", "/", nil)
	startc := make(chan struct{})
	req = req.WithContext(context.WithValue(req.Context(), testHandlerStartKey{}, startc))
	finishc := make(chan struct{})
	req = req.WithContext(context.WithValue(req.Context(), testHandlerFinishKey{}, finishc))

	var success, overload uint32

	hnd := httpgovernor.New(httpgovernor.Params{
		MaxConcurrency: 1,
	}, testHandler)
	var wg1 sync.WaitGroup
	wg1.Add(1)
	go doReq(wg1.Done, hnd, req, &success, &overload)
	// Ensure the first handler is running.
	<-startc
	// Start 2 more requests.
	var wg2 sync.WaitGroup
	wg2.Add(2)
	go doReq(wg2.Done, hnd, req, &success, &overload)
	go doReq(wg2.Done, hnd, req, &success, &overload)
	// Wait for the requests to stop.
	wg2.Wait()
	// Finish the first request.
	close(finishc)
	// Wait for all requests to finish.
	wg1.Wait()

	c.Assert(atomic.LoadUint32(&success), qt.Equals, uint32(1))
	c.Assert(atomic.LoadUint32(&overload), qt.Equals, uint32(2))
}

func TestSimpleGovernorWithCounter(t *testing.T) {
	c := qt.New(t)

	req := httptest.NewRequest("", "/", nil)
	startc := make(chan struct{})
	req = req.WithContext(context.WithValue(req.Context(), testHandlerStartKey{}, startc))
	finishc := make(chan struct{})
	req = req.WithContext(context.WithValue(req.Context(), testHandlerFinishKey{}, finishc))

	var success, overload uint32
	var overloadc testValue

	hnd := httpgovernor.New(httpgovernor.Params{
		MaxConcurrency:         1,
		RequestOverloadCounter: &overloadc,
	}, testHandler)
	var wg1 sync.WaitGroup
	wg1.Add(1)
	go doReq(wg1.Done, hnd, req, &success, &overload)
	// Ensure the first handler is running.
	<-startc
	// Start 2 more requests.
	var wg2 sync.WaitGroup
	wg2.Add(2)
	go doReq(wg2.Done, hnd, req, &success, &overload)
	go doReq(wg2.Done, hnd, req, &success, &overload)
	// Wait for the requests to stop.
	wg2.Wait()
	// Finish the first request.
	close(finishc)
	// Wait for all requests to finish.
	wg1.Wait()

	c.Assert(atomic.LoadUint32(&success), qt.Equals, uint32(1))
	c.Assert(atomic.LoadUint32(&overload), qt.Equals, uint32(2))
	c.Assert(overloadc.Int32(), qt.Equals, int32(2))
}

func TestSimpleGovernorWithZeroCostRequests(t *testing.T) {
	c := qt.New(t)

	req := httptest.NewRequest("", "/", nil)
	startc := make(chan struct{})
	req = req.WithContext(context.WithValue(req.Context(), testHandlerStartKey{}, startc))
	finishc := make(chan struct{})
	req = req.WithContext(context.WithValue(req.Context(), testHandlerFinishKey{}, finishc))

	var success, overload uint32

	hnd := httpgovernor.New(httpgovernor.Params{
		MaxConcurrency: 1,
		CostEstimator:  httpgovernor.PathCostEstimator{"/free": 0},
	}, testHandler)
	var wg1 sync.WaitGroup
	wg1.Add(1)
	go doReq(wg1.Done, hnd, req, &success, &overload)
	// Ensure the first handler is running.
	<-startc
	var wg2 sync.WaitGroup
	// Start 2 more requests.
	req2 := httptest.NewRequest("", "/free", nil)
	wg2.Add(2)
	go doReq(wg2.Done, hnd, req2, &success, &overload)
	go doReq(wg2.Done, hnd, req, &success, &overload)
	wg2.Wait()
	// Finish the first request.
	close(finishc)
	// Wait for all requests to finish.
	wg1.Wait()

	c.Assert(atomic.LoadUint32(&success), qt.Equals, uint32(2))
	c.Assert(atomic.LoadUint32(&overload), qt.Equals, uint32(1))
}

func TestQueuingGovernor(t *testing.T) {
	c := qt.New(t)

	req := httptest.NewRequest("", "/", nil)
	startc := make(chan struct{})
	req = req.WithContext(context.WithValue(req.Context(), testHandlerStartKey{}, startc))
	finishc := make(chan struct{})
	req = req.WithContext(context.WithValue(req.Context(), testHandlerFinishKey{}, finishc))

	var success, overload uint32

	hnd := httpgovernor.New(httpgovernor.Params{
		MaxConcurrency: 1,
		MaxBurst:       2,
	}, testHandler)
	var wg1 sync.WaitGroup
	wg1.Add(1)
	go doReq(wg1.Done, hnd, req, &success, &overload)
	// Ensure the first handler is running.
	<-startc
	// Start 2 more requests.
	var wg2 sync.WaitGroup
	wg2.Add(2)
	ch := make(chan struct{}, 1)
	done := func() {
		wg2.Done()
		ch <- struct{}{}
	}
	go doReq(done, hnd, req, &success, &overload)
	go doReq(done, hnd, req, &success, &overload)
	// Wait unit one of the new requests is complete.
	<-ch
	// Complete the first request handler.
	close(finishc)
	// Wait for the second request handler to start.
	<-startc
	// Wait for all handlers to complete.
	wg2.Wait()
	wg1.Wait()

	c.Assert(atomic.LoadUint32(&success), qt.Equals, uint32(2))
	c.Assert(atomic.LoadUint32(&overload), qt.Equals, uint32(1))
}

func TestQueuingGovernorWithTimeout(t *testing.T) {
	c := qt.New(t)

	req := httptest.NewRequest("", "/", nil)
	startc := make(chan struct{})
	req = req.WithContext(context.WithValue(req.Context(), testHandlerStartKey{}, startc))
	finishc := make(chan struct{})
	req = req.WithContext(context.WithValue(req.Context(), testHandlerFinishKey{}, finishc))

	var success, overload uint32

	hnd := httpgovernor.New(httpgovernor.Params{
		MaxConcurrency:   1,
		MaxBurst:         2,
		MaxQueueDuration: 100 * time.Microsecond,
	}, testHandler)
	var wg1 sync.WaitGroup
	wg1.Add(1)
	go doReq(wg1.Done, hnd, req, &success, &overload)
	// Ensure the first handler is running.
	<-startc
	// Start 2 more requests.
	var wg2 sync.WaitGroup
	wg2.Add(2)
	go doReq(wg2.Done, hnd, req, &success, &overload)
	go doReq(wg2.Done, hnd, req, &success, &overload)
	// Wait for the new requests to finish.
	wg2.Wait()
	// Complete the first request.
	close(finishc)
	wg1.Wait()

	c.Assert(atomic.LoadUint32(&success), qt.Equals, uint32(1))
	c.Assert(atomic.LoadUint32(&overload), qt.Equals, uint32(2))
}

func TestQueuingGovernorWithCounter(t *testing.T) {
	c := qt.New(t)

	req := httptest.NewRequest("", "/", nil)
	startc := make(chan struct{})
	req = req.WithContext(context.WithValue(req.Context(), testHandlerStartKey{}, startc))
	finishc := make(chan struct{})
	req = req.WithContext(context.WithValue(req.Context(), testHandlerFinishKey{}, finishc))

	var success, overload uint32
	var overloadc testValue

	hnd := httpgovernor.New(httpgovernor.Params{
		MaxConcurrency:         1,
		MaxBurst:               2,
		MaxQueueDuration:       100 * time.Microsecond,
		RequestOverloadCounter: &overloadc,
	}, testHandler)
	var wg1 sync.WaitGroup
	wg1.Add(1)
	go doReq(wg1.Done, hnd, req, &success, &overload)
	// Ensure the first handler is running.
	<-startc
	// Wait for the first request to start.
	var wg2 sync.WaitGroup
	// Start 2 more requests.
	wg2.Add(2)
	go doReq(wg2.Done, hnd, req, &success, &overload)
	go doReq(wg2.Done, hnd, req, &success, &overload)
	// Wait for the 2 new requests to finish.
	wg2.Wait()
	// Complete the first request
	close(finishc)
	wg1.Wait()

	c.Assert(atomic.LoadUint32(&success), qt.Equals, uint32(1))
	c.Assert(atomic.LoadUint32(&overload), qt.Equals, uint32(2))
	c.Assert(overloadc.Int32(), qt.Equals, int32(2))
}

func TestQueueingGovernorWithZeroCostRequests(t *testing.T) {
	c := qt.New(t)

	req := httptest.NewRequest("", "/", nil)
	startc := make(chan struct{})
	req = req.WithContext(context.WithValue(req.Context(), testHandlerStartKey{}, startc))
	finishc := make(chan struct{})
	req = req.WithContext(context.WithValue(req.Context(), testHandlerFinishKey{}, finishc))

	var success, overload uint32

	hnd := httpgovernor.New(httpgovernor.Params{
		MaxConcurrency: 1,
		MaxBurst:       2,
		CostEstimator:  httpgovernor.PathCostEstimator{"/free": 0},
	}, testHandler)
	var wg1 sync.WaitGroup
	wg1.Add(1)
	go doReq(wg1.Done, hnd, req, &success, &overload)
	// Ensure the first handler is running.
	<-startc
	var wg2 sync.WaitGroup
	wg2.Add(3)
	ch := make(chan struct{}, 1)
	done := func() {
		wg2.Done()
		ch <- struct{}{}
	}
	// Start 2 more requests.
	go doReq(done, hnd, req, &success, &overload)
	go doReq(done, hnd, req, &success, &overload)
	// Wait for one of the requests to complete.
	<-ch
	// Start a 0 cost request.
	go doReq(done, hnd, httptest.NewRequest("", "/free", nil), &success, &overload)
	// Wait for the request to complete.
	<-ch
	// Finish all requests.
	close(finishc)
	// Wait for all requests to complete.
	<-startc
	wg2.Wait()
	wg1.Wait()

	c.Assert(atomic.LoadUint32(&success), qt.Equals, uint32(3))
	c.Assert(atomic.LoadUint32(&overload), qt.Equals, uint32(1))
}

func TestQueuingGovernorWithGauges(t *testing.T) {
	c := qt.New(t)

	req := httptest.NewRequest("", "/", nil)
	startc := make(chan struct{})
	req = req.WithContext(context.WithValue(req.Context(), testHandlerStartKey{}, startc))
	finishc := make(chan struct{})
	req = req.WithContext(context.WithValue(req.Context(), testHandlerFinishKey{}, finishc))

	var success, overload uint32
	var qgauge testValue
	var observer testObserver

	hnd := httpgovernor.New(httpgovernor.Params{
		MaxConcurrency:        1,
		MaxBurst:              2,
		QueueLengthGauge:      &qgauge,
		QueueDurationObserver: &observer,
	}, testHandler)
	var wg1 sync.WaitGroup
	wg1.Add(1)
	go doReq(wg1.Done, hnd, req, &success, &overload)
	// Ensure the first handler is running.
	<-startc
	var wg2 sync.WaitGroup
	wg2.Add(2)
	ch := make(chan struct{}, 1)
	done := func() {
		wg2.Done()
		ch <- struct{}{}
	}
	// Start 2 more requests.
	go doReq(done, hnd, req, &success, &overload)
	go doReq(done, hnd, req, &success, &overload)
	// Wait for one of the requests to complete.
	<-ch
	// Check the queue is currently 1 item long.
	c.Check(qgauge.Int32(), qt.Equals, int32(1))
	// Complete the first request.
	close(finishc)
	// Wait for the second request to start.
	<-startc
	// Ensure the first request has finished.
	wg1.Wait()
	// Check the queue is currently 0 items long.
	c.Check(qgauge.Int32(), qt.Equals, int32(0))
	// Wait for all requests to complete.
	wg2.Wait()

	c.Assert(atomic.LoadUint32(&success), qt.Equals, uint32(2))
	c.Assert(atomic.LoadUint32(&overload), qt.Equals, uint32(1))
	c.Assert(observer.count, qt.Equals, 1)
}

type testHandlerStartKey struct{}
type testHandlerFinishKey struct{}

var testHandler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
	if c, ok := req.Context().Value(testHandlerStartKey{}).(chan struct{}); ok {
		c <- struct{}{}
	}
	if c, ok := req.Context().Value(testHandlerFinishKey{}).(chan struct{}); ok {
		<-c
	}
	w.Write([]byte("OK"))
})

func doReq(done func(), hnd http.Handler, req *http.Request, success, overload *uint32) {
	defer done()
	rr := httptest.NewRecorder()
	hnd.ServeHTTP(rr, req)
	resp := rr.Result()
	if resp.StatusCode == http.StatusServiceUnavailable {
		atomic.AddUint32(overload, 1)
	} else {
		atomic.AddUint32(success, 1)
	}
}

type testValue int32

func (v *testValue) Inc() {
	atomic.AddInt32((*int32)(v), 1)
}

func (v *testValue) Dec() {
	atomic.AddInt32((*int32)(v), -1)
}

func (v *testValue) Int32() int32 {
	return atomic.LoadInt32((*int32)(v))
}

type testObserver struct {
	mu    sync.Mutex
	count int
	value float64
}

func (o *testObserver) Observe(v float64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.count++
	o.value = v
}
