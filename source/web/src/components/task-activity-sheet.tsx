"use client";

import * as React from "react";
import Link from "next/link";
import { ArrowUpRightIcon, RefreshCwIcon, RadioIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { api, sseUrl } from "@/lib/api";
import { MOCK } from "@/lib/mock/enabled";
import type { Activity, Task } from "@/lib/types";

const PAGE_SIZE = 200;
const MAX_ITEMS = 400;
type Category = "all" | "agent" | "tool" | "llm" | "rag" | "approval";

function categoryOf(item: Activity): Category {
  if (item.kind === "intercept_request") return "approval";
  if ((item.kind === "tool_use" || item.kind === "tool_result") && item.tool?.toLowerCase() === "rag_search") {
    return "rag";
  }
  if (item.kind === "tool_use" || item.kind === "tool_result") return "tool";
  if (item.kind === "thinking" || item.kind === "usage" || item.kind === "llm_switch" || item.kind === "llm_failover") {
    return "llm";
  }
  return "agent";
}

const categoryLabels: Record<Category, string> = {
  all: "全部",
  agent: "Agent",
  tool: "工具",
  llm: "模型事件",
  rag: "RAG",
  approval: "审批",
};

function mergeActivities(previous: Activity[], incoming: Activity[]): Activity[] {
  if (!incoming.length) return previous;
  const bySeq = new Map(previous.map((item) => [item.seq, item]));
  for (const item of incoming) bySeq.set(item.seq, item);
  return [...bySeq.values()].sort((a, b) => a.seq - b.seq).slice(-MAX_ITEMS);
}

export function TaskActivitySheet({ task, onClose }: { task: Task | null; onClose: () => void }) {
  return (
    <Sheet open={task !== null} onOpenChange={(open) => { if (!open) onClose(); }}>
      {task && <TaskActivityContent key={task.id} task={task} />}
    </Sheet>
  );
}

function TaskActivityContent({ task }: { task: Task }) {
  const [items, setItems] = React.useState<Activity[]>([]);
  const [category, setCategory] = React.useState<Category>("all");
  const [worker, setWorker] = React.useState("all");
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState("");
  const [connected, setConnected] = React.useState(false);
  const [retry, setRetry] = React.useState(0);
  const [hasOlder, setHasOlder] = React.useState(false);

  // biome-ignore lint/correctness/useExhaustiveDependencies: retry explicitly restarts the snapshot and stream.
  React.useEffect(() => {
    let active = true;
    let stream: EventSource | null = null;
    setLoading(true);
    setError("");
    setConnected(false);
    setItems([]);
    const snapshot = MOCK
      ? api.activity(task.id).then((page) => ({ items: page.items.slice(-PAGE_SIZE), hasMore: page.items.length > PAGE_SIZE, snapshotCursor: page.cursor }))
      : api.activityHistory(task.id, "all", 0, PAGE_SIZE);
    snapshot
      .then((page) => {
        if (!active) return;
        setItems(page.items);
        setHasOlder(page.hasMore);
        setLoading(false);
        if (MOCK) return;
        stream = new EventSource(
          sseUrl(`/api/exploration/activity/stream?task=${encodeURIComponent(task.id)}&since=${page.snapshotCursor}`),
        );
        stream.onopen = () => { if (active) setConnected(true); };
        stream.onerror = () => { if (active) setConnected(false); };
        stream.onmessage = (event) => {
          try {
            const item = JSON.parse(event.data) as Activity;
            if (active && typeof item.seq === "number") setItems((current) => mergeActivities(current, [item]));
          } catch {
            // A malformed frame must not interrupt the stream.
          }
        };
      })
      .catch((cause: Error) => {
        if (!active) return;
        setLoading(false);
        setError(cause.message === "bad session" ? "后端尚未加载任务级活动接口，请在可控窗口重启服务。" : cause.message || "活动加载失败");
      });
    return () => {
      active = false;
      stream?.close();
    };
  }, [task.id, retry]);

  const workers = React.useMemo(() => [...new Set(items.map((item) => item.worker).filter(Boolean))].sort(), [items]);
  const visible = React.useMemo(
    () => items.filter((item) => (category === "all" || categoryOf(item) === category) &&
      (worker === "all" || item.worker === worker)).reverse(),
    [items, category, worker],
  );
  const detailUrl = `/function/tasks/detail?id=${encodeURIComponent(task.id)}`;
  let streamLabel = "未连接";
  if (MOCK) streamLabel = "示例";
  else if (connected) streamLabel = "实时";
  else if (loading) streamLabel = "连接中";

  return (
    <SheetContent className="grid h-full w-full! max-w-none! grid-rows-[auto_minmax(0,1fr)] gap-0 overflow-hidden p-0 sm:w-[42rem]! sm:max-w-[42rem]!">
      <SheetHeader className="border-b px-5 py-4 pr-12">
        <div className="flex items-center gap-2">
          <SheetTitle className="min-w-0 truncate">任务 #{task.id} · 执行流水</SheetTitle>
          <span className={`flex shrink-0 items-center gap-1 text-xs ${connected ? "text-emerald-600" : "text-muted-foreground"}`}>
            <RadioIcon className="size-3.5" />
            {streamLabel}
          </span>
        </div>
        <SheetDescription className="truncate">{task.name?.trim() ? task.name : task.description}</SheetDescription>
        <div className="flex flex-wrap items-center gap-2 pt-3">
          <Select value={category} onValueChange={(value) => setCategory(value as Category)}>
            <SelectTrigger className="w-28" aria-label="事件类型"><SelectValue /></SelectTrigger>
            <SelectContent>
              {(Object.keys(categoryLabels) as Category[]).map((value) =>
                <SelectItem key={value} value={value}>{categoryLabels[value]}</SelectItem>)}
            </SelectContent>
          </Select>
          <Select value={worker} onValueChange={setWorker}>
            <SelectTrigger className="w-36" aria-label="Agent"><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部 Agent</SelectItem>
              {workers.map((name) => <SelectItem key={name} value={name}>{name}</SelectItem>)}
            </SelectContent>
          </Select>
          <Button size="icon-sm" variant="ghost" title="重新连接" aria-label="重新连接" onClick={() => setRetry((value) => value + 1)}>
            <RefreshCwIcon />
          </Button>
          <Button size="sm" variant="outline" asChild className="ml-auto">
            <Link href={detailUrl}>任务详情 <ArrowUpRightIcon /></Link>
          </Button>
        </div>
      </SheetHeader>
      <ScrollArea className="min-h-0">
        {!loading && !error && <p className="border-b px-5 py-2 text-muted-foreground text-xs">最近活动 · 模型请求正文和流量证据如有保存，请在独立模块核验</p>}
        {loading && <p className="p-5 text-muted-foreground text-sm">正在加载活动…</p>}
        {!loading && error && <p role="alert" className="p-5 text-destructive text-sm">{error}</p>}
        {!loading && !error && (
          <div className="divide-y px-5 pb-6">
            {visible.length === 0 && <p className="py-8 text-center text-muted-foreground text-sm">暂无匹配的活动</p>}
            {visible.map((item) => {
              const ref = item.metadata?.record_ref;
              return (
                <div key={item.seq} className="py-3 text-xs">
                  <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-muted-foreground">
                    <time title={item.ts} className="tabular-nums">
                      {item.ts && !Number.isNaN(Date.parse(item.ts)) ? new Date(item.ts).toLocaleString("zh-CN") : "—"}
                    </time>
                    <span className="font-medium text-foreground">{item.worker || "系统"}</span>
                    <span>{categoryLabels[categoryOf(item)]} · {item.tool || item.kind}</span>
                    {item.is_error && <span className="text-destructive">失败</span>}
                  </div>
                  <p className="mt-1 break-words text-sm">{item.summary || "无摘要"}</p>
                  {item.metadata?.llm_request_id && (
                    <Link
                      className="mt-1 block break-all text-muted-foreground underline underline-offset-2"
                      href={`/function/llm-records?task=${encodeURIComponent(task.id)}&request_id=${encodeURIComponent(item.metadata.llm_request_id)}`}
                    >
                      模型请求 {item.metadata.llm_request_id}
                    </Link>
                  )}
                  {ref && (
                    <p className="mt-1 text-muted-foreground">
                      <Link
                        className="underline underline-offset-2 hover:text-foreground"
                        href={ref.kind === "finding"
                          ? `/function/findings/detail?id=${encodeURIComponent(ref.id)}&context_task=${encodeURIComponent(task.id)}`
                          : ref.kind === "rag_proposal"
                            ? `/system/rag?knowledge_base_id=${encodeURIComponent(ref.knowledge_base_id ?? "")}&tab=proposals#proposal-${encodeURIComponent(ref.id)}`
                            : `${detailUrl}&tab=context#retrieval-${encodeURIComponent(ref.id)}`}
                      >
                        {ref.kind === "finding" ? "Finding" : ref.kind === "rag_proposal" ? "知识提案" : "检索日志"} #{ref.id}
                      </Link>
                      {ref.kind === "rag_retrieval" && " · 工具返回，不代表已进入模型请求"}
                    </p>
                  )}
                </div>
              );
            })}
            {(hasOlder || items.length === MAX_ITEMS) && <p className="py-3 text-muted-foreground text-xs">更早记录请查看任务详情。</p>}
          </div>
        )}
      </ScrollArea>
    </SheetContent>
  );
}
