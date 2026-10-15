package server

import "testing"

func TestParseActivitySessionAll(t *testing.T) {
	filter, ok := parseActivitySession("all")
	if !ok || filter.Main || filter.Worker != "" || filter.NodeID != nil || filter.MainSeg != nil {
		t.Fatalf("all should select the whole task: filter=%+v ok=%v", filter, ok)
	}
}
