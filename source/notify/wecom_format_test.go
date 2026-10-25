package notify

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestWeComVerifiedAlertHasContextWithoutHTTPSecrets(t *testing.T) {
	m := singleMsg()
	m.Items[0].Kind = EventFindingVerified
	m.Items[0].TaskID = 73
	m.Items[0].Assets = []string{"https://target.example.com/login?token=SECRET_TARGET"}
	m.Items[0].Summary = "可绕过鉴权\nCookie: SID=SECRET_COOKIE Authorization: Bearer SECRET_BEARER"
	body, kept := weComBody(m)
	for _, text := range []string{"已验证漏洞", "高危", "target.example.com", "SQL注入", "任务：#73", "漏洞：#42", "证据与影响详见平台", "[在平台查看详情]"} {
		if !strings.Contains(body, text) {
			t.Errorf("缺少 %q: %s", text, body)
		}
	}
	for _, secret := range []string{"SECRET_COOKIE", "SECRET_BEARER", "SECRET_TARGET", "Cookie:", "Authorization:"} {
		if strings.Contains(body, secret) {
			t.Errorf("泄露 %q: %s", secret, body)
		}
	}
	if kept != 1 || len(body) > weComMarkdownLimit {
		t.Fatalf("企微消息超限或未计入投递: bytes=%d kept=%d", len(body), kept)
	}
}

func TestWeComStatusChangeAndDigestSplitWholeEntries(t *testing.T) {
	status := singleMsg()
	status.Items[0].Kind = EventFindingStatusChanged
	status.Items[0].FromStatus, status.Items[0].ToStatus = "pending", "fixed"
	body, _ := weComBody(status)
	if !strings.Contains(body, "处置状态更新") || !strings.Contains(body, "待处理 → 已修复") {
		t.Fatalf("状态变化没有独立标题与结果: %s", body)
	}
	batch := batchMsg(200)
	for i := range batch.Items {
		batch.Items[i].Kind = EventFindingVerified
		batch.Items[i].TaskID = 73
	}
	body, kept := weComBody(batch)
	if kept < 1 || kept >= len(batch.Items) || len(body) > weComMarkdownLimit || !utf8.ValidString(body) {
		t.Fatalf("汇总拆分不正确: kept=%d bytes=%d", kept, len(body))
	}
	if !strings.Contains(body, "近 30 分钟") || !strings.Contains(body, "其余") || !strings.Contains(body, "已验证漏洞") {
		t.Fatalf("汇总缺少时间窗、进度或事件标签: %s", body)
	}
	if !strings.Contains(body, "漏洞"+itoa(kept)) || strings.Contains(body, "漏洞"+itoa(kept+1)+" ·") {
		t.Fatalf("汇总整条边界不正确: kept=%d", kept)
	}
}

func TestWeComHTTPPayloadUsesCompactFormat(t *testing.T) {
	srv := capturePost(t, `{"errcode":0,"errmsg":"ok"}`, func(t *testing.T, body map[string]any, _ *http.Request) {
		md, _ := body["markdown"].(map[string]any)
		content, _ := md["content"].(string)
		if !strings.Contains(content, "已验证漏洞") || !strings.Contains(content, "任务：#73") {
			t.Fatalf("发送的内容未采用新版格式: %s", content)
		}
	})
	m := singleMsg()
	m.Items[0].Kind = EventFindingVerified
	m.Items[0].TaskID = 73
	if n, err := (weComChannel{}).Send(context.Background(), map[string]any{"webhook": srv.URL}, m); err != nil || n != 1 {
		t.Fatalf("投递失败: n=%d err=%v", n, err)
	}
}
