// Copyright 2020 Canonical Ltd.

package httpgovernor_test

import (
	"net/http"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/juju/httpgovernor"
)

var _ httpgovernor.CostEstimator = (*httpgovernor.PatternCostEstimator)(nil)

var pathCostTests = []struct {
	name       string
	costs      map[string]int64
	host       string
	path       string
	expectCost int64
}{{
	name:       "empty",
	path:       "/",
	expectCost: 1,
}, {
	name: "exact_path",
	costs: map[string]int64{
		"/":     10,
		"/free": 0,
		"/api":  5,
	},
	path:       "/api",
	expectCost: 5,
}, {
	name: "zero_cost",
	costs: map[string]int64{
		"/":     10,
		"/free": 0,
		"/api":  5,
	},
	path:       "/free",
	expectCost: 0,
}, {
	name: "prefix_match",
	costs: map[string]int64{
		"/":     10,
		"/free": 0,
		"/api/": 5,
	},
	path:       "/api/call",
	expectCost: 5,
}, {
	name: "no_match",
	costs: map[string]int64{
		"/free": 0,
		"/api":  5,
	},
	path:       "/nomatch",
	expectCost: 1,
}, {
	name: "host_match",
	costs: map[string]int64{
		"test2.example.com": 10,
	},
	host:       "test2.example.com",
	path:       "/",
	expectCost: 10,
}, {
	name: "host_path",
	costs: map[string]int64{
		"test2.example.com/free": 0,
		"test2.example.com/api/": 5,
	},
	host:       "test2.example.com",
	path:       "/api/call",
	expectCost: 5,
}, {
	name: "host_not_matched",
	costs: map[string]int64{
		"test2.example.com/free": 0,
		"test2.example.com/api":  5,
	},
	path:       "/api/call",
	expectCost: 1,
}, {
	name: "ip_host",
	costs: map[string]int64{
		"127.0.0.1/free": 0,
		"127.0.0.1/api/": 5,
	},
	host:       "127.0.0.1",
	path:       "/api/call",
	expectCost: 5,
}, {
	name: "ip6_host",
	costs: map[string]int64{
		"[::1]/free": 0,
		"[::1]/api/": 5,
	},
	host:       "[::1]",
	path:       "/api/call",
	expectCost: 5,
}, {
	name: "ip_hostport",
	costs: map[string]int64{
		"127.0.0.1/free": 0,
		"127.0.0.1/api/": 5,
	},
	host:       "127.0.0.1:80",
	path:       "/api/call",
	expectCost: 5,
}, {
	name: "ip6_hostport",
	costs: map[string]int64{
		"[::1]/free": 0,
		"[::1]/api/": 5,
	},
	host:       "[::1]:80",
	path:       "/api/call",
	expectCost: 5,
}, {
	name: "longest_match",
	costs: map[string]int64{
		"/free":       0,
		"/api/":       5,
		"/api/calls/": 7,
	},
	host:       "[::1]:80",
	path:       "/api/calls/call1",
	expectCost: 7,
}}

func TestPathEstimator(t *testing.T) {
	c := qt.New(t)

	for _, test := range pathCostTests {
		c.Run(test.name, func(c *qt.C) {
			pce := new(httpgovernor.PatternCostEstimator)
			for path, cost := range test.costs {
				pce.SetCost(path, cost)
			}
			if test.host == "" {
				test.host = "test.example.com"
			}
			req, err := http.NewRequest("GET", "http://"+test.host+test.path, nil)
			c.Assert(err, qt.IsNil)
			cost := pce.EstimateCost(req)
			c.Check(cost, qt.Equals, test.expectCost)
		})
	}
}

func TestSetCostMultiple(t *testing.T) {
	c := qt.New(t)

	pce := new(httpgovernor.PatternCostEstimator)
	pce.SetCost("/api/", 5)
	pce.SetCost("/api/", 7)
	req, err := http.NewRequest("GET", "http://example.com/api/call", nil)
	c.Assert(err, qt.IsNil)
	c.Check(pce.EstimateCost(req), qt.Equals, int64(7))
}
