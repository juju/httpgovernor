// Copyright 2020 Canonical Ltd.

package httpgovernor

import (
	"net/http"
	stdpath "path"
	"strings"
	"sync"
)

// A PattenCostEstimator determines the cost of a request by matching the
// request against a list of configured patterns.
//
// The supported patterns are similar to the ones used in http.ServeMux.
// A path must either be a rooted path, or a rooted subtree. As in
// http.ServeMux the longest match takes precedence.
//
// A pattern may include a host before the path. If a host is specified
// only requests addressed to that host will be matched. Any
// host-specific match will take precedence over all-host matches.
type PatternCostEstimator struct {
	// mu is used to protect the fields in this structure.
	mu sync.RWMutex

	// costs contains the costs of paths supported by this estimator.
	costs map[string]int64

	// prefixes contain a list of prefixes that might be matched to
	// identify costs. These are stored in order, longest to shortest
	// so that more specific matches will be matched first.
	prefixes []string

	// hasHost stores whether any of the costs contain a host part.
	// This allows the matcher to skip checking for matches with a
	// host part, if it wouldn't match anything anyway.
	hasHost bool
}

// EstimateCost determines the cost of the given request by matching in
// the PatternCostEstimator. Any path not known is assumed to have a
// cost of 1.
func (c *PatternCostEstimator) EstimateCost(req *http.Request) int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	path := stdpath.Clean(req.URL.Path)
	if c.hasHost {
		host := stripPort(req.Host)
		cost, ok := c.match(host + path)
		if ok {
			return cost
		}
	}

	cost, _ := c.match(path)
	return cost
}

// stripPort removes a port from the http.Request.Host parameter, if
// present.
func stripPort(hostport string) string {
	n := strings.LastIndexByte(hostport, ':')
	if n == -1 {
		return hostport
	}
	if hostport[0] == '[' && hostport[n-1] != ']' {
		return hostport
	}
	return hostport[:n]
}

// match is used to match the given path (which might include a host) to
// a cost. match should only be called with a read lock held.
func (c *PatternCostEstimator) match(path string) (int64, bool) {
	// first look for an exact match.
	cost, ok := c.costs[path]
	if ok {
		return cost, true
	}

	// look for the longest matching prefix.
	for _, prefix := range c.prefixes {
		if strings.HasPrefix(path, prefix) {
			return c.costs[prefix], true
		}
	}

	// return the default cost of 1.
	return 1, false
}

// SetCost configures the cost of a matched pattern.
func (c *PatternCostEstimator) SetCost(path string, cost int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.costs == nil {
		c.costs = make(map[string]int64)
	}

	var host string
	n := strings.Index(path, "/")
	switch n {
	case -1:
		host = path
		path = "/"
	case 0:
		break
	default:
		host = path[:n]
		path = path[n:]
	}

	prefix := strings.HasSuffix(path, "/")
	path = stdpath.Clean(path)
	if prefix && !strings.HasSuffix(path, "/") {
		// Re-add the trailing slash
		path += "/"
	}
	cleanPath := host + path

	if host != "" {
		c.hasHost = true
	}
	if prefix {
		c.addPrefix(cleanPath)
	}
	c.costs[cleanPath] = cost
}

// addPrefix adds the prefix to the list of prefixes that will be matched
// to a request. addPrefix expects to be called with the write lock held.
func (c *PatternCostEstimator) addPrefix(prefix string) {
	for i, p := range c.prefixes {
		if p == prefix {
			return
		}
		if len(prefix) > len(p) {
			c.prefixes = append(c.prefixes, "")
			copy(c.prefixes[i+1:], c.prefixes[i:])
			c.prefixes[i] = prefix
			return
		}
	}

	// If we make it this far the prefix has to go on the end.
	c.prefixes = append(c.prefixes, prefix)
}
