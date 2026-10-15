"use client";

import * as React from "react";

import Link from "next/link";

import { BookOpenIcon, RefreshCwIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { api } from "@/lib/api";
import type { RAGPromptInclusion, RAGRetrievalLog, Task } from "@/lib/types";

export function ContextReferencesTab({ taskId }: { taskId: string }) {
  const [logs, setLogs] = React.useState<RAGRetrievalLog[]>([]);
  const [metrics, setMetrics] = React.useState<{
    latest_llm: Task["latest_llm"] | null;
    rag_stats: NonNullable<Task["rag_stats"]>;
    prompt_inclusions: RAGPromptInclusion[];
  } | null>(null);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState("");
  const [revision, setRevision] = React.useState(0);

  // biome-ignore lint/correctness/useExhaustiveDependencies: revision explicitly refreshes the snapshot.
  React.useEffect(() => {
    let active = true;
    setLoading(true);
    Promise.all([api.taskRAGRetrievalLogs(taskId, 100), api.taskRuntimeMetrics(taskId)])
      .then(([nextLogs, nextMetrics]) => {
        if (!active) return;
        setLogs(nextLogs);
        setMetrics(nextMetrics);
        setError("");
      })
      .catch((cause: Error) => {
        if (active) setError(cause.message || "记录加载失败");
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [taskId, revision]);

  const latest = metrics?.latest_llm;
  const windowTokens = latest?.context_window_tokens ?? 0;
  const watermark = latest && windowTokens > 0 && latest.input_tokens > 0
    ? Math.round((latest.input_tokens / windowTokens) * 100)
    : null;
  const stats = metrics?.rag_stats;
  const included = React.useMemo(() => {
    const byChunk = new Map<string, RAGPromptInclusion[]>();
    for (const item of metrics?.prompt_inclusions ?? []) {
      const key = `${item.retrieval_log_id}:${item.chunk_id}`;
      byChunk.set(key, [...(byChunk.get(key) ?? []), item]);
    }
    return byChunk;
  }, [metrics]);
  React.useEffect(() => {
    if (loading || error || !logs.length || !window.location.hash.startsWith("#retrieval-")) return;
    document.getElementById(window.location.hash.slice(1))?.scrollIntoView({ block: "center" });
  }, [loading, error, logs]);
  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <div className="flex items-start justify-between gap-4 border-b pb-4">
        <div>
          <h2 className="flex items-center gap-2 font-semibold text-base">
            <BookOpenIcon className="size-4" /> 上下文与引用
          </h2>
          <p className="mt-1 text-muted-foreground text-xs">
            检索返回与请求消息包含分别标注；包含只证明切片文本在该次请求的工具结果中，不代表模型采纳或内容正确。
          </p>
        </div>
        <Button
          size="icon"
          variant="ghost"
          title="刷新"
          aria-label="刷新上下文与引用"
          onClick={() => setRevision((n) => n + 1)}
        >
          <RefreshCwIcon />
        </Button>
      </div>
      {error && (
        <p role="alert" className="text-destructive text-sm">
          {error}
        </p>
      )}
      {loading && <p className="text-muted-foreground text-sm">加载中…</p>}
      {!loading && !error && (
        <>
          <section className="grid gap-3 border-b pb-5 text-sm sm:grid-cols-3">
            <div>
              <p className="text-muted-foreground text-xs">最近模型请求输入</p>
              <p className="mt-1 font-medium tabular-nums">{latest ? latest.input_tokens.toLocaleString() : "未知"}</p>
            </div>
            <div>
              <p className="text-muted-foreground text-xs">上下文水位</p>
              <p className={`mt-1 font-medium ${watermark !== null && watermark >= 85 ? "text-destructive" : ""}`}>
                {watermark === null ? "未知" : `${watermark}%`}
              </p>
              <p className="text-muted-foreground text-xs">
                {windowTokens > 0 ? `人工确认窗口 ${windowTokens.toLocaleString()} tokens` : "未核实模型窗口大小"}
              </p>
            </div>
            <div>
              <p className="text-muted-foreground text-xs">非空检索 / 总检索</p>
              <p className="mt-1 font-medium tabular-nums">
                {stats?.nonempty ?? 0} / {stats?.searches ?? 0}
              </p>
              <p className="text-muted-foreground text-xs">共返回 {stats?.returned_chunks ?? 0} 个切片</p>
            </div>
            {latest && (
              <p className="min-w-0 break-all text-muted-foreground text-xs sm:col-span-3">
                最近请求: {latest.request_id || "历史记录无 ID"} · 会话 {latest.session_id || "未知"} ·{" "}
                {latest.worker || "Agent"} · {latest.model || "未知模型"}
              </p>
            )}
          </section>
          <section className="space-y-3">
            <h3 className="font-semibold text-sm">检索记录</h3>
            {logs.length === 0 && <p className="text-muted-foreground text-sm">暂无检索记录</p>}
            {logs.map((log) => (
              <article key={log.id} id={`retrieval-${log.id}`} className="scroll-mt-20 border-b pb-4 last:border-0">
                <div className="flex flex-wrap gap-x-3 gap-y-1 text-muted-foreground text-xs">
                  <time>{new Date(log.created_at).toLocaleString("zh-CN")}</time>
                  <span>日志 #{log.id}</span>
                  <span>知识库 #{log.knowledge_base_id}</span>
                  <span>Agent: {log.agent_id || "未知"}</span>
                  <span>
                    {log.mode} · Top-{log.top_k} · 返回 {log.result_count} · {log.latency_ms} ms
                  </span>
                </div>
                <p className="mt-1 break-words font-medium text-sm">{log.query}</p>
                <div className="mt-2 space-y-2">
                  {log.results.map((result, index) => (
                    <details key={`${log.id}-${result.chunk_id}`} className="rounded border px-3 py-2 text-sm">
                      <summary className="cursor-pointer break-words">
                        {index + 1}. {result.document_name || `文档 #${result.document_id}`} · 切片 #{result.chunk_id}
                        {Number.isFinite(result.score) ? ` · 排序分数 ${result.score.toFixed(4)}` : ""}
                        {included.has(`${log.id}:${result.chunk_id}`) ? " · 请求消息已包含" : " · 仅证实检索返回"}
                      </summary>
                      <div className="mt-2 space-y-1 text-xs">
                        {(included.get(`${log.id}:${result.chunk_id}`) ?? []).map((item) => (
                          <p key={item.request_id} className="break-all text-muted-foreground">
                            请求 {item.request_id} · 工具调用 {item.tool_use_id || "未知"}
                          </p>
                        ))}
                        {result.heading_path?.length > 0 && (
                          <p className="break-words text-muted-foreground">{result.heading_path.join(" / ")}</p>
                        )}
                        {result.source_uri && <p className="break-all text-muted-foreground">{result.source_uri}</p>}
                        {(result.page_from > 0 || result.line_from > 0) && (
                          <p className="text-muted-foreground">
                            {result.page_from > 0
                              ? `第 ${result.page_from}-${result.page_to || result.page_from} 页`
                              : `第 ${result.line_from}-${result.line_to || result.line_from} 行`}
                          </p>
                        )}
                        <p className="whitespace-pre-wrap break-words">{result.content}</p>
                      </div>
                    </details>
                  ))}
                  {log.result_count === 0 && <p className="text-muted-foreground text-xs">无返回切片</p>}
                </div>
              </article>
            ))}
            {logs.length === 100 && <p className="text-muted-foreground text-xs">仅展示最近 100 次检索。</p>}
          </section>
          <p className="text-muted-foreground text-xs">
            历史请求和截断前的切片可能没有包含索引；完整模型请求正文仅在启用录制时可供核验。
            <Link
              href={
                latest?.request_id
                  ? `/function/llm-records?task=${encodeURIComponent(taskId)}&request_id=${encodeURIComponent(latest.request_id)}`
                  : `/function/llm-records?task=${encodeURIComponent(taskId)}`
              }
              className="underline"
            >
              查看 LLM 录制
            </Link>
          </p>
        </>
      )}
    </div>
  );
}
