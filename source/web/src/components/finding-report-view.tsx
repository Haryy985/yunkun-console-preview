"use client";

import * as React from "react";

import { DownloadIcon, EyeIcon, RefreshCwIcon } from "lucide-react";
import { toast } from "sonner";

import { HttpCodeBlock } from "@/components/http-code-block";
import { Markdown } from "@/components/markdown";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { api } from "@/lib/api";
import { statusMeta } from "@/lib/status";
import type { Finding, FindingTraffic, FindingTrafficDetail } from "@/lib/types";

const roles: Record<string, string> = {
  baseline: "正常基线",
  proof: "漏洞证明",
  verification: "复核验证",
  supporting: "辅助证据",
};

function redactRequestHead(head: string) {
  return head.replace(
    /^([ \t]*(?:Authorization|Proxy-Authorization|Cookie|X-Api-Key|X-Auth-Token)[ \t]*:).+$/gim,
    "$1 [REDACTED]",
  );
}

function ReportField({ label, value }: { label: string; value?: string }) {
  return (
    <div className="min-w-0 border-b py-3">
      <dt className="mb-1 text-xs text-muted-foreground">{label}</dt>
      <dd className="break-words text-sm">{value?.trim() || "未记录"}</dd>
    </div>
  );
}

export function FindingReportView({ finding, contextTask }: { finding: Finding; contextTask?: string }) {
  const [traffic, setTraffic] = React.useState<FindingTraffic | null>(null);
  const [selected, setSelected] = React.useState("");
  const [detail, setDetail] = React.useState<FindingTrafficDetail | null>(null);
  const [error, setError] = React.useState("");
  const [loadingMore, setLoadingMore] = React.useState(false);
  const [showRequestBody, setShowRequestBody] = React.useState(false);

  React.useEffect(() => {
    let active = true;
    setTraffic(null);
    setDetail(null);
    setSelected("");
    setError("");
    api
      .findingTraffic(finding.id, contextTask)
      .then((result) => {
        if (!active) return;
        setTraffic(result);
        setSelected(result.bindings[0]?.id ?? "");
      })
      .catch((e: Error) => {
        if (active) setError(e.message);
      });
    return () => {
      active = false;
    };
  }, [finding.id, contextTask]);

  React.useEffect(() => {
    if (!selected) return;
    let active = true;
    setDetail(null);
    setError("");
    setShowRequestBody(false);
    api
      .findingTrafficDetail(finding.id, selected, contextTask)
      .then((result) => {
        if (active) setDetail(result);
      })
      .catch((e: Error) => {
        if (active) setError(e.message);
      });
    return () => {
      active = false;
    };
  }, [finding.id, selected, contextTask]);

  async function loadMore(side: "request" | "response") {
    if (!detail) return;
    setLoadingMore(true);
    try {
      const next = await api.findingTrafficBody(
        finding.id,
        detail.binding.id,
        side,
        detail[side].next_offset,
        contextTask,
      );
      setDetail((current) =>
        current?.binding.id === detail.binding.id
          ? { ...current, [side]: { ...next, content: current[side].content + next.content } }
          : current,
      );
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setLoadingMore(false);
    }
  }

  const location = traffic?.bindings.find((b) => b.snapshot.url)?.snapshot.url;
  const legacyCurl = /(?:^|\n)\s*(?:\$\s*)?curl(?:\.exe)?\s/im.test(finding.report ?? "");
  let reportContent: React.ReactNode = <p className="text-sm text-muted-foreground">详细分析与修复建议待补充。</p>;
  if (legacyCurl) {
    reportContent = (
      <Alert>
        <AlertDescription>旧报告仍包含 curl 复现，尚不符合交付格式。请依据上方实际请求包重新撰写。</AlertDescription>
      </Alert>
    );
  } else if (finding.report) {
    reportContent = <Markdown text={finding.report} />;
  }

  return (
    <div className="mx-auto max-w-5xl space-y-8 pb-12">
      <section>
        <h2 className="border-b pb-3 text-base font-semibold">漏洞信息</h2>
        <dl className="grid gap-x-8 sm:grid-cols-2">
          <ReportField
            label="所属项目"
            value={finding.task_description?.trim() || (finding.task_id ? `任务 #${finding.task_id}` : "")}
          />
          <ReportField label="测试目标" value={location ?? finding.assets?.[0]?.label} />
          <ReportField label="漏洞名称" value={finding.name?.trim() ? finding.name : finding.vulnclass} />
          <ReportField label="漏洞位置" value={location} />
          <ReportField label="漏洞分类" value={finding.vulnclass} />
          <ReportField label="所处阶段" value="无法确定" />
          <ReportField label="风险等级" value={`${statusMeta("severity", finding.severity).label}（以人工复核为准）`} />
          <ReportField label="处理状态" value={statusMeta("finding", finding.status).label} />
        </dl>
      </section>

      <section className="space-y-3">
        <h2 className="border-b pb-3 text-base font-semibold">漏洞描述</h2>
        <p className="text-sm leading-7 whitespace-pre-wrap">{finding.summary || "尚未填写经验证的描述。"}</p>
      </section>

      <section className="space-y-4">
        <h2 className="border-b pb-3 text-base font-semibold">复现过程 · HTTP 请求包</h2>
        {error && (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        {!traffic && !error && <Skeleton className="h-44 w-full" />}
        {traffic && traffic.bindings.length === 0 && (
          <Alert>
            <AlertDescription>
              未绑定可核实的 HTTP 请求包。不能用 curl 命令或 URL 推测请求；非 HTTP 漏洞请查看原始执行证据。
            </AlertDescription>
          </Alert>
        )}
        {traffic && traffic.bindings.length > 0 && (
          <>
            <fieldset className="flex flex-wrap gap-2">
              <legend className="sr-only">选择流量证据</legend>
              {traffic.bindings.map((binding, index) => (
                <Button
                  key={binding.id}
                  size="sm"
                  variant={selected === binding.id ? "default" : "outline"}
                  onClick={() => setSelected(binding.id)}
                >
                  {index + 1}. {roles[binding.role] ?? binding.role}
                </Button>
              ))}
            </fieldset>
            {!detail ? (
              <Skeleton className="h-52 w-full" />
            ) : (
              <div className="space-y-4">
                <p className="break-all text-xs text-muted-foreground">
                  证据 #{detail.binding.id} · {detail.binding.snapshot.method} {detail.binding.snapshot.url}
                </p>
                {detail.binding.note && <p className="text-sm">{detail.binding.note}</p>}
                <div className="space-y-2">
                  <h3 className="text-sm font-medium">请求</h3>
                  <p className="text-xs text-muted-foreground">
                    凭据请求头已遮盖。正文可能包含账号或个人数据，展开及下载原始证据前请确认使用范围。
                  </p>
                  {detail.request.binary ? (
                    <p className="text-sm text-muted-foreground">二进制正文未内嵌；请核验完整证据附件。</p>
                  ) : (
                    <HttpCodeBlock
                      raw={`${redactRequestHead(detail.binding.snapshot.req_head?.trimEnd() ?? "")}\n\n${showRequestBody || detail.request.total === 0 ? detail.request.content : "[请求正文已隐藏]"}`}
                    />
                  )}
                  {!detail.request.binary && !showRequestBody && detail.request.total > 0 && (
                    <Button size="sm" variant="outline" onClick={() => setShowRequestBody(true)}>
                      <EyeIcon /> 显示原始正文
                    </Button>
                  )}
                  {detail.request.truncated && !detail.request.binary && showRequestBody && (
                    <Button size="sm" variant="outline" disabled={loadingMore} onClick={() => void loadMore("request")}>
                      <RefreshCwIcon /> 加载剩余请求正文
                    </Button>
                  )}
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() =>
                      void api
                        .downloadFindingTrafficBody(finding.id, detail.binding.id, "request", contextTask)
                        .catch((e: Error) => toast.error(e.message))
                    }
                  >
                    <DownloadIcon /> 下载原始请求正文
                  </Button>
                </div>
                <details className="border-t pt-3">
                  <summary className="cursor-pointer text-sm font-medium">响应与验证结果</summary>
                  {detail.response.binary ? (
                    <p className="pt-2 text-sm text-muted-foreground">二进制响应正文未内嵌。</p>
                  ) : (
                    <div className="pt-3">
                      <HttpCodeBlock
                        raw={`${detail.binding.snapshot.resp_head?.trimEnd() ?? ""}\n\n${detail.response.content}`}
                      />
                    </div>
                  )}
                  {detail.response.truncated && !detail.response.binary && (
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={loadingMore}
                      onClick={() => void loadMore("response")}
                    >
                      <RefreshCwIcon /> 加载剩余响应正文
                    </Button>
                  )}
                </details>
              </div>
            )}
          </>
        )}
      </section>

      <section className="space-y-3">
        <h2 className="border-b pb-3 text-base font-semibold">影响与修复建议</h2>
        {finding.report_stale && (
          <Alert>
            <AlertDescription>证据已变更，报告需要复核。</AlertDescription>
          </Alert>
        )}
        {reportContent}
      </section>
    </div>
  );
}
