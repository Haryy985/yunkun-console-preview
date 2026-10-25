package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"yunkun/console/db"
)

func TestIntentAssetPayloadFailsClosed(t *testing.T) {
	for _, payload := range []string{
		`{"asset_ids":"1"}`,
		`{"asset_ids":["1"]}`,
		`{"asset_ids":{}}`,
		`{"asset_ids":[1,2.5]}`,
	} {
		intent := &db.Node{Payload: json.RawMessage(payload)}
		if _, err := intentAssetIDs(intent); err == nil {
			t.Fatalf("malformed payload accepted: %s", payload)
		}
		w := &Worker{}
		if _, _, err := w.Execute(context.Background(), "worker", 1, nil, nil, intent, nil, nil, nil, nil); err == nil {
			t.Fatalf("worker accepted malformed payload: %s", payload)
		}
	}
	for _, payload := range []string{`{"summary":"recon"}`, `{"asset_ids":[]}`} {
		ids, err := intentAssetIDs(&db.Node{Payload: json.RawMessage(payload)})
		if err != nil || len(ids) != 0 {
			t.Fatalf("unanchored intent: ids=%v err=%v", ids, err)
		}
	}
	if _, _, err := (&Worker{}).Execute(context.Background(), "worker", 1, nil, nil,
		&db.Node{Payload: json.RawMessage(`{"asset_ids":[1]}`)}, nil, nil, nil, nil); err == nil {
		t.Fatal("worker accepted target ids without a task asset store")
	}
}

func TestIntentAssetScopeIsolation(t *testing.T) {
	dsn := os.Getenv("YUNKUN_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("requires a disposable YUNKUN_TEST_PG_DSN; never use an application database")
	}
	d, err := db.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	current, err := d.CreateTask("scope current", "test", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := d.CreateTask("scope foreign", "test", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.DeleteTask(current.ID); _ = d.DeleteTask(foreign.ID) })
	as := d.Assets()
	stamp := time.Now().UnixNano()
	currentID, err := as.UpsertRootDomain(db.UpsertRootDomainReq{Domain: fmt.Sprintf("current-%d.invalid", stamp), TaskID: current.ID})
	if err != nil {
		t.Fatal(err)
	}
	foreignID, err := as.UpsertRootDomain(db.UpsertRootDomainReq{Domain: fmt.Sprintf("foreign-%d.invalid", stamp), TaskID: foreign.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = as.DeleteByIDs([]int64{currentID, foreignID}) })

	tools := NewToolSet(d.Exploration(current.ExplorationID), "planner")
	tools.SetTaskID(current.ID)
	tools.SetAssetStore(as, as.Companies())
	idJSON := func(id int64) json.RawMessage { return json.RawMessage(fmt.Sprintf("%d", id)) }
	for _, id := range []int64{foreignID, -1, 999999999} {
		if _, err := tools.addOneIntent(intentItem{Summary: "test", AssetIDs: []json.RawMessage{idJSON(id)}}); err == nil {
			t.Fatalf("out-of-scope or invalid id %d accepted", id)
		}
	}
	if _, err := tools.addOneIntent(intentItem{Summary: "test", AssetIDs: []json.RawMessage{idJSON(currentID), idJSON(foreignID)}}); err == nil {
		t.Fatal("mixed batch accepted a foreign asset")
	}
	if _, err := tools.addOneIntent(intentItem{Summary: "test", AssetIDs: []json.RawMessage{json.RawMessage(`"bad"`)}}); err == nil {
		t.Fatal("malformed asset id accepted")
	}
	if _, err := tools.addOneIntent(intentItem{Summary: "test", AssetIDs: []json.RawMessage{idJSON(currentID)}}); err != nil {
		t.Fatalf("own task asset rejected: %v", err)
	}

	foreignIntent := &db.Node{ID: 1, Payload: json.RawMessage(fmt.Sprintf(`{"asset_ids":[%d]}`, foreignID))}
	if _, _, err := (&Worker{}).Execute(context.Background(), "worker", current.ID, as, nil, foreignIntent, nil, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "授权范围") {
		t.Fatalf("worker did not refuse an old foreign intent: %v", err)
	}

	// A human's explicit attachment is a valid authorization edge, even when
	// the enterprise's automatic attribution has no matching scope rule.
	if _, err := as.AttachAssetsToTask(current.ID, []int64{foreignID}, "test operator approval"); err != nil {
		t.Fatal(err)
	}
	if _, err := tools.addOneIntent(intentItem{Summary: "test", AssetIDs: []json.RawMessage{idJSON(foreignID)}}); err != nil {
		t.Fatalf("explicitly attached asset rejected: %v", err)
	}
}
