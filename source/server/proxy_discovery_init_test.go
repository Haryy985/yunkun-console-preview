package server

import (
	"sync"
	"testing"
)

func TestProxyDiscoveryConcurrentInitialization(t *testing.T) {
	s := &Server{}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.initProxyDiscovery()
		}()
	}
	wg.Wait()
	if s.proxyDiscovery == nil || s.proxyDiscovery.client == nil {
		t.Fatal("discovery service was not initialized")
	}
}
