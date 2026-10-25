package notify

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

// Group messages deliberately contain no raw HTTP evidence. The destination
// receives a short alert; the authenticated console remains the evidence store.
var sensitiveImpact = regexp.MustCompile(`(?i)(cookie|authorization|bearer|token|secret|password|passwd|api.?key|set-cookie|http/[0-9]|(?:^|\s)(?:get|post|put|delete)\s+/|[?&][^=\s]+=[^\s&]+)`)
var safeHost = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,252}$`)

func weComEventLabel(it Item) string {
	switch it.Kind {
	case EventFindingVerified:
		return "已验证漏洞"
	case EventFindingStatusChanged:
		return "处置状态更新"
	case EventFindingCreated:
		return "新发现线索"
	default:
		return "漏洞通知"
	}
}

func weComTarget(assets []string) string {
	if len(assets) == 0 {
		return ""
	}
	var hosts []string
	for _, asset := range assets {
		asset = strings.TrimSpace(asset)
		if strings.Contains(asset, "://") {
			u, err := url.Parse(asset)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
				continue
			}
			asset = u.Hostname()
		} else if strings.ContainsAny(asset, "/?#@\\ \t\r\n") {
			continue
		}
		if !safeHost.MatchString(asset) || net.ParseIP(asset) == nil && strings.Contains(asset, ":") {
			continue
		}
		hosts = append(hosts, asset)
		if len(hosts) == 2 {
			break
		}
	}
	return strings.Join(hosts, "、")
}

func weComImpact(summary string) string {
	summary = OneLine(summary, 80)
	if summary == "" || sensitiveImpact.MatchString(summary) {
		return "证据与影响详见平台"
	}
	return markdownText(summary, 80)
}

func weComItem(it Item, index int, compact bool) string {
	var b strings.Builder
	if compact {
		fmt.Fprintf(&b, "%d. **%s · %s**", index, SeverityLabel(it.Severity), markdownText(it.Title(), 56))
		fmt.Fprintf(&b, " · %s", weComEventLabel(it))
		if it.TaskID > 0 {
			fmt.Fprintf(&b, " · 任务 #%d", it.TaskID)
		}
		if host := weComTarget(it.Assets); host != "" {
			fmt.Fprintf(&b, "\n   目标：%s", markdownText(host, 0))
		}
		b.WriteString("\n")
		return b.String()
	}
	fmt.Fprintf(&b, "**%s · %s**\n", weComEventLabel(it), SeverityLabel(it.Severity))
	fmt.Fprintf(&b, "**%s**\n", markdownText(it.Title(), 80))
	if host := weComTarget(it.Assets); host != "" {
		fmt.Fprintf(&b, "目标：%s\n", markdownText(host, 0))
	}
	if it.VulnClass != "" {
		fmt.Fprintf(&b, "类型：%s\n", markdownText(it.VulnClass, 50))
	}
	fmt.Fprintf(&b, "影响：%s\n", weComImpact(it.Summary))
	if it.IsStatusChange() {
		fmt.Fprintf(&b, "处置：%s → %s\n",
			markdownText(StatusLabel(it.FromStatus), 24), markdownText(StatusLabel(it.ToStatus), 24))
	}
	if it.TaskID > 0 {
		fmt.Fprintf(&b, "任务：#%d  ", it.TaskID)
	}
	fmt.Fprintf(&b, "漏洞：#%d\n", it.FindingID)
	if it.DetailURL != "" && len(it.DetailURL) <= 600 {
		fmt.Fprintf(&b, "[在平台查看详情](%s)\n", it.DetailURL)
	}
	return b.String()
}

func weComDigest(m Message, kept int) string {
	var b strings.Builder
	if m.WindowMinutes > 0 {
		fmt.Fprintf(&b, "**近 %d 分钟漏洞动态 · %d 条**\n", m.WindowMinutes, len(m.Items))
	} else {
		fmt.Fprintf(&b, "**漏洞动态 · %d 条**\n", len(m.Items))
	}
	counts := map[string]int{}
	for _, it := range m.Items[:kept] {
		counts[it.Severity]++
	}
	var parts []string
	for _, severity := range []string{"critical", "high", "medium", "low"} {
		if n := counts[severity]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", SeverityLabel(severity), n))
		}
	}
	if len(parts) > 0 {
		b.WriteString(strings.Join(parts, " · ") + "\n")
	}
	if rest := len(m.Items) - kept; rest > 0 {
		fmt.Fprintf(&b, "本条 %d 条，其余 %d 条将在下一条消息继续\n", kept, rest)
	}
	b.WriteString("\n")
	for i, it := range m.Items[:kept] {
		b.WriteString(weComItem(it, i+1, true))
	}
	if m.HomeURL != "" && len(m.HomeURL) <= 600 {
		fmt.Fprintf(&b, "\n[在平台中查看全部](%s)\n", m.HomeURL)
	}
	return b.String()
}

func weComBody(m Message) (string, int) {
	if len(m.Items) == 0 {
		return "", 0
	}
	if !m.Batch {
		body := weComItem(m.Items[0], 0, false)
		if len(body) > weComMarkdownLimit {
			body = fmt.Sprintf("**%s · %s**\n漏洞 #%d · 详情请在平台查看",
				weComEventLabel(m.Items[0]), SeverityLabel(m.Items[0].Severity), m.Items[0].FindingID)
		}
		return body, 1
	}
	// Render the real header/footer at each step: counting just item bytes can
	// truncate an included row while still marking it delivered.
	kept := 1
	for n := 1; n <= len(m.Items); n++ {
		if len(weComDigest(m, n)) > weComMarkdownLimit {
			break
		}
		kept = n
	}
	body := weComDigest(m, kept)
	if len(body) > weComMarkdownLimit {
		// A single pathological row must not stall the entire delivery queue.
		it := m.Items[0]
		body = fmt.Sprintf("**漏洞动态 · %d 条**\n本条 1 条，其余 %d 条将在下一条消息继续\n漏洞 #%d · %s",
			len(m.Items), len(m.Items)-1, it.FindingID, SeverityLabel(it.Severity))
	}
	return body, kept
}
