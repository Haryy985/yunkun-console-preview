// Package report renders a deliverable Markdown report from the exploration
// graph: confirmed findings (with severity/evidence) (docs §15 P5).
package report

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"yunkun/console/db"
)

// Input bundles what the report needs.
type Input struct {
	Title       string
	Goal        string
	GeneratedAt time.Time
	AssetCounts map[string]int
	Findings    []*db.Node
	References  map[int64]db.TaskReportReference
	Requests    map[int64]RequestPacket // binding ID -> captured request
}

type findingView struct {
	NodeID    int64
	VulnClass string
	Name      string
	Severity  string
	Summary   string
	PoC       string
}

func parseFinding(n *db.Node) findingView {
	var p struct {
		VulnClass string `json:"vulnclass"`
		Name      string `json:"name"`
		Severity  string `json:"severity"`
		Summary   string `json:"summary"`
		Evidence  struct {
			PoC string `json:"poc"`
		} `json:"evidence"`
	}
	_ = json.Unmarshal(n.Payload, &p)
	return findingView{n.ID, p.VulnClass, p.Name, p.Severity, p.Summary, p.Evidence.PoC}
}

var sevRank = map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3, "": 4}

// Markdown renders the report.
func Markdown(in Input) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# 渗透测试报告 — %s\n\n", nz(in.Title, "未命名任务"))
	fmt.Fprintf(&b, "- **任务目标**：%s\n", nz(in.Goal, "（未指定）"))
	fmt.Fprintf(&b, "- **生成时间**：%s\n\n", in.GeneratedAt.Format("2006-01-02 15:04:05"))

	// summary
	fmt.Fprintf(&b, "## 摘要\n\n")
	fmt.Fprintf(&b, "- 漏洞记录（以人工复核为准）：**%d** 个\n", len(in.Findings))
	fmt.Fprintf(&b, "- 资产：")
	var types []string
	for t := range in.AssetCounts {
		types = append(types, t)
	}
	sort.Strings(types)
	for i, t := range types {
		if i > 0 {
			b.WriteString("、")
		}
		fmt.Fprintf(&b, "%s %d", t, in.AssetCounts[t])
	}
	b.WriteString("\n\n")

	// findings
	fmt.Fprintf(&b, "## 发现\n\n")
	if len(in.Findings) == 0 {
		b.WriteString("_本次未确认漏洞。_\n\n")
	} else {
		fs := make([]findingView, 0, len(in.Findings))
		for _, n := range in.Findings {
			fs = append(fs, parseFinding(n))
		}
		sort.SliceStable(fs, func(i, j int) bool { return sevRank[fs[i].Severity] < sevRank[fs[j].Severity] })
		for i, f := range fs {
			fmt.Fprintf(&b, "### %d. [%s] %s\n\n", i+1, strings.ToUpper(nz(f.Severity, "info")), nz(f.Name, nz(f.VulnClass, "未分类")))
			fmt.Fprintf(&b, "%s\n\n", nz(f.Summary, ""))
			if f.PoC != "" {
				b.WriteString(findingEvidenceMarkdown(f.PoC))
			}
			if ref, ok := in.References[f.NodeID]; ok {
				fmt.Fprintf(&b, "**Finding 记录：** #%d（节点 #%d）\n\n", ref.FindingID, f.NodeID)
				if strings.TrimSpace(ref.Report) != "" {
					if ref.EvidenceVersion != ref.ReportEvidenceVersion {
						b.WriteString("**注意：详细报告引用的证据版本已变化，需要复核。**\n\n")
					}
					fmt.Fprintf(&b, "%s\n\n", reportBodyForExport(ref.Report))
				}
				if len(ref.Traffic) > 0 {
					b.WriteString("**流量证据引用与 HTTP 请求包：**\n\n")
					for _, item := range ref.Traffic {
						fmt.Fprintf(&b, "- 绑定 #%d（%s）：快照 `%s`，源流量 `%s`\n",
							item.BindingID, item.Role, item.SnapshotID, item.SourceTrafficID)
						b.WriteString("\n")
						b.WriteString(requestPacketMarkdown(in.Requests[item.BindingID]))
					}
					b.WriteString("\n")
				}
			}
		}
	}

	return b.String()
}

func nz(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}
