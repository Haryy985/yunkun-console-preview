package server

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestDiscoveryScopeNormalization(t *testing.T) {
	for _, test := range []struct {
		scope string
		want  string
	}{
		{"", ""},
		{" 0.0.0.0 ", "0.0.0.0/0"},
		{"0.0.0.0; ::/0\nExample.COM", "0.0.0.0/0,::/0,example.com"},
		{"203.0.113.7,198.51.100.0/24", "203.0.113.7,198.51.100.0/24"},
	} {
		if got := normalizeDiscoveryScope(test.scope); got != test.want {
			t.Errorf("normalize %q: got %q, want %q", test.scope, got, test.want)
		}
	}
}

func TestDiscoveryScopeMatching(t *testing.T) {
	for _, test := range []struct {
		host, scope string
		want        bool
	}{
		{"203.0.113.5", "0.0.0.0", true},
		{"198.51.100.1", "0.0.0.0/0", true},
		{"2001:db8::1", "0.0.0.0", false},
		{"2001:db8::1", "::/0", true},
		{"203.0.113.5", "::/0", false},
		{"203.0.113.5", "", false},
		{"203.0.113.5", "203.0.113.6", false},
		{"203.0.113.5", "203.0.113.5", true},
		{"203.0.113.5", "203.0.113.0/24", true},
		{"203.0.114.5", "203.0.113.0/24", false},
		{"Sub.Example.com.", "example.com", true},
		{"notexample.com", "example.com", false},
		{"example.com.attacker.invalid", "example.com", false},
		{"example.com", "0.0.0.0", false},
	} {
		if got := hostInScope(test.host, test.scope); got != test.want {
			t.Errorf("host %q scope %q: got %v, want %v", test.host, test.scope, got, test.want)
		}
	}
}

func TestDiscoveryBatchValidation(t *testing.T) {
	ids := make([]int64, 101)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	for _, test := range []struct {
		name string
		req  discoveryBatchRequest
		ok   bool
	}{
		{"verify", discoveryBatchRequest{Action: "verify", IDs: ids[:100]}, true},
		{"verify too many", discoveryBatchRequest{Action: "verify", IDs: ids}, false},
		{"delete", discoveryBatchRequest{Action: "delete", IDs: ids[:100]}, true},
		{"delete too many", discoveryBatchRequest{Action: "delete", IDs: ids}, false},
		{"empty", discoveryBatchRequest{Action: "delete"}, false},
		{"zero", discoveryBatchRequest{Action: "delete", IDs: []int64{0}}, false},
		{"negative", discoveryBatchRequest{Action: "delete", IDs: []int64{-1}}, false},
		{"duplicate", discoveryBatchRequest{Action: "delete", IDs: []int64{1, 1}}, false},
		{"unknown", discoveryBatchRequest{Action: "enable", IDs: ids[:1]}, false},
		{"missing pool", discoveryBatchRequest{Action: "approve", IDs: ids[:1], Target: "candidate"}, false},
		{"missing target", discoveryBatchRequest{Action: "approve", IDs: ids[:1], PoolID: 1}, false},
		{"invalid target", discoveryBatchRequest{Action: "approve", IDs: ids[:1], PoolID: 1, Target: "bad"}, false},
		{"disabled admission", discoveryBatchRequest{Action: "approve", IDs: ids[:1], PoolID: 1, Target: "candidate"}, true},
		{"enabled admission", discoveryBatchRequest{Action: "approve", IDs: ids[:100], PoolID: 1, Target: "production"}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := append([]int64(nil), test.req.IDs...)
			err := test.req.validate()
			if (err == nil) != test.ok {
				t.Fatalf("validation: %v, want success %v", err, test.ok)
			}
			if !reflect.DeepEqual(before, test.req.IDs) {
				t.Fatal("validation mutated the requested IDs")
			}
		})
	}
}

func TestDiscoveryVerifySlotLimitsConcurrencyAndRefills(t *testing.T) {
	d := &proxyDiscoveryService{}
	var wg sync.WaitGroup
	starts := make(chan int, 11)
	releaseOne := make(chan struct{})
	releaseAll := make(chan struct{})
	begin := make(chan struct{})
	for i := 0; i < 11; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			<-begin
			release, err := d.acquireVerifySlot(context.Background())
			if err != nil {
				t.Error(err)
				return
			}
			starts <- id
			select {
			case <-releaseOne:
			case <-releaseAll:
			}
			release()
		}(i)
	}
	close(begin)
	deadline := time.After(700 * time.Millisecond)
	for i := 0; i < 10; i++ {
		select {
		case <-starts:
		case <-deadline:
			close(releaseAll)
			wg.Wait()
			t.Fatal("first ten verification slots did not start together")
		}
	}
	select {
	case <-starts:
		close(releaseAll)
		wg.Wait()
		t.Fatal("more than ten verifications ran concurrently")
	default:
	}
	releaseOne <- struct{}{}
	select {
	case <-starts:
	case <-time.After(700 * time.Millisecond):
		close(releaseAll)
		wg.Wait()
		t.Fatal("waiting verifier did not start after release")
	}
	close(releaseAll)
	wg.Wait()
}

func TestDiscoveryVerifySlotCancellation(t *testing.T) {
	d := &proxyDiscoveryService{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	release, err := d.acquireVerifySlot(ctx)
	if err == nil {
		release()
		t.Fatal("canceled verification acquired a slot")
	}
}
