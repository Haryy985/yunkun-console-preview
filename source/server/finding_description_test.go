package server

import (
	"testing"

	"yunkun/console/agent"
)

func TestFindingDescriptionUpdatesOldDefaultOnly(t *testing.T) {
	s, _, _ := trafficEvidenceServer(t)
	pg := s.m.pg
	original, err := pg.GetTool("report_finding")
	if err != nil || original == nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pg.Exec(`UPDATE tools SET description=$1 WHERE key='report_finding'`, original.Description)
	})
	if _, err := pg.Exec(`UPDATE tools SET description=$1 WHERE key='report_finding'`, previousFindingDescription); err != nil {
		t.Fatal(err)
	}
	s.refreshDefaultFindingDescription()
	s.refreshDefaultFindingDescription()
	after, err := pg.GetTool("report_finding")
	if err != nil || after == nil {
		t.Fatal(err)
	}
	want := agent.NewToolSet(nil, "").MainAgentTools()
	var defaultDescription string
	for _, tool := range want {
		if tool.Name() == "report_finding" {
			defaultDescription = tool.Description()
		}
	}
	if defaultDescription == "" || after.Description != defaultDescription {
		t.Fatal("previous default did not migrate to the current description")
	}
	if _, err := pg.Exec(`UPDATE tools SET description=$1 WHERE key='report_finding'`, "operator-customized"); err != nil {
		t.Fatal(err)
	}
	s.refreshDefaultFindingDescription()
	after, err = pg.GetTool("report_finding")
	if err != nil || after == nil || after.Description != "operator-customized" {
		t.Fatal("administrator description was overwritten")
	}
}
