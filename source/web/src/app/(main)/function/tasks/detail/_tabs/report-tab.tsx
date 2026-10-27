"use client";

import * as React from "react";

import { CheckIcon, CopyIcon, FileTextIcon, RotateCwIcon } from "lucide-react";
import { toast } from "sonner";

import { Markdown } from "@/components/markdown";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { api } from "@/lib/api";
import { ReportRequestError } from "@/lib/report-request";
import { useRetryCountdown } from "@/hooks/use-retry-countdown";
import { copyText } from "@/lib/utils";

export function ReportTab({ taskId }: { taskId: string }) {
  const [report, setReport] = React.useState<string>("");
  const [error, setError] = React.useState<string>("");
  const [loading, setLoading] = React.useState(true);
  const [copied, setCopied] = React.useState(false);
  const [attempt, setAttempt] = React.useState(0);
  const [retryAt, setRetryAt] = React.useState(0);
  const retrySeconds = useRetryCountdown(retryAt);
  const retrying = React.useRef(false);

  React.useEffect(() => {
    let active = true;
    const controller = new AbortController();
    setLoading(true);
    setError("");
    setReport("");
    setCopied(false);
    api
      .report(taskId, controller.signal)
      .then((text) => {
        if (active) {
          setReport(text);
          setRetryAt(0);
        }
      })
      .catch((reason) => {
        if (active) {
          setReport("");
          setError((reason as Error).message);
          setRetryAt(reason instanceof ReportRequestError ? reason.retryAt : 0);
        }
      })
      .finally(() => {
        if (active) {
          setLoading(false);
          retrying.current = false;
        }
      });
    return () => {
      active = false;
      controller.abort();
    };
  }, [taskId, attempt]);

  function retry() {
    if (loading || retrying.current || Date.now() < retryAt) return;
    retrying.current = true;
    setAttempt((value) => value + 1);
  }

  async function copy() {
    if (!report) return;
    const ok = await copyText(report);
    if (ok) {
      setCopied(true);
      toast.success("已复制 Markdown");
      setTimeout(() => setCopied(false), 1500);
    } else {
      toast.error("复制失败，请手动选择文本复制");
    }
  }

  let content: React.ReactNode;
  if (loading) {
    content = (
      <div className="flex flex-col items-center justify-center gap-2 rounded-md border border-dashed py-16 text-muted-foreground text-sm">
        <FileTextIcon className="size-8 opacity-40" />
        加载中…
      </div>
    );
  } else if (report) {
    content = (
      <div className="max-h-[60vh] overflow-auto rounded-md border bg-muted/20 p-4">
        <Markdown text={report} />
      </div>
    );
  } else if (error) {
    content = (
      <p role="alert" className="max-h-64 overflow-auto whitespace-pre-wrap break-all py-8 text-destructive text-sm">
        {error}
      </p>
    );
  } else {
    content = (
      <div className="flex flex-col items-center justify-center gap-2 rounded-md border border-dashed py-16 text-muted-foreground text-sm">
        <FileTextIcon className="size-8 opacity-40" />
        暂无报告
      </div>
    );
  }

  return (
    <Card>
      <CardHeader className="flex-row flex-wrap items-center justify-between gap-2">
        <CardTitle className="flex items-center gap-2 text-sm">
          <FileTextIcon className="size-4" /> 渗透测试报告（Markdown）
        </CardTitle>
        <div className="flex gap-2">
          {error && (
            <Button size="sm" variant="outline" onClick={retry} disabled={loading || retrySeconds > 0}>
              <RotateCwIcon /> {retrySeconds > 0 ? `${retrySeconds} 秒后重试` : "重试"}
            </Button>
          )}
          {report && (
            <Button size="sm" variant="outline" onClick={copy}>
              {copied ? <CheckIcon /> : <CopyIcon />} 复制
            </Button>
          )}
        </div>
      </CardHeader>
      <CardContent>{content}</CardContent>
    </Card>
  );
}
