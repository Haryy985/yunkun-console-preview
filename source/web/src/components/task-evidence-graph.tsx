"use client";
"use no memo";

import * as React from "react";
import Link from "next/link";
import {
  Background,
  BackgroundVariant,
  Controls,
  MarkerType,
  ReactFlow,
  type Edge,
  type Node,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { ExternalLinkIcon, RefreshCwIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { api } from "@/lib/api";
import type { Activity, Finding, FindingTraffic, RAGPromptInclusion, RAGRetrievalLog, Task } from "@/lib/types";

type EvidenceNode = Node<{ label: string; href?: string; detail?: string }>;
type EvidenceGraph = { nodes: EvidenceNode[]; edges: Edge[] };

const NODE_WIDTH = 208;
const ROW_HEIGHT = 92;
const MAX_ACTIVITIES = 80;
const MAX_LOGS = 12;

function buildEvidenceGraph(task: Task, activities: Activity[], logs: RAGRetrievalLog[], findings: Finding[], traffic: FindingTraffic[], inclusions: RAGPromptInclusion[]): EvidenceGraph {
  const nodes: EvidenceNode[] = [];
  const edges: Edge[] = [];
  const counts = new Map<number, number>();
  const known = new Set<string>();
  const calls = new Map<string, string>();
  const logByID = new Map(logs.map((log) => [String(log.id), log]));
  const taskNode = `task:${task.id}`;

  function add(id: string, label: string, column: number, detail?: string, href?: string) {
    if (known.has(id)) return;
    known.add(id);
    const row = counts.get(column) ?? 0;
    counts.set(column, row + 1);
    nodes.push({
      id,
      position: { x: column * 264, y: row * ROW_HEIGHT },
      data: { label, detail, href },
      style: {
        width: NODE_WIDTH,
        minHeight: 58,
        borderRadius: 6,
        border: "1px solid var(--border)",
        background: "var(--background)",
        color: "var(--foreground)",
        fontSize: 12,
        whiteSpace: "normal",
        overflowWrap: "anywhere",
      },
    });
  }

  function connect(source: string, target: string, label: string) {
    const id = `${source}:${target}:${label}`;
    if (edges.some((edge) => edge.id === id)) return;
    edges.push({
      id, source, target, label,
      markerEnd: { type: MarkerType.ArrowClosed },
      style: { stroke: "#64748b" },
      labelStyle: { fontSize: 10, fill: "#64748b" },
    });
  }

  function addRetrieval(id: string, source: string, relation: string) {
    const log = logByID.get(id);
    const logNode = `retrieval:${id}`;
    add(logNode, `检索 #${id}${log ? ` · ${log.query.slice(0, 48)}` : ""}`, 3,
      log ? `工具返回 ${log.result_count} 个切片；不等于进入模型 Prompt` : "本页未加载到该检索记录",
      `/function/tasks/detail?id=${encodeURIComponent(task.id)}&tab=context#retrieval-${encodeURIComponent(id)}`);
    connect(source, logNode, relation);
    if (!log) return;
    const kbNode = `kb:${log.knowledge_base_id}`;
    add(kbNode, `知识库 #${log.knowledge_base_id}`, 4);
    connect(logNode, kbNode, "检索自");
    for (const result of log.results.slice(0, 5)) {
      const docNode = `doc:${result.document_id}`;
      const chunkNode = `chunk:${result.chunk_id}`;
      add(docNode, `文档 ${result.document_name || `#${result.document_id}`}`, 5, result.source_uri);
      add(chunkNode, `切片 #${result.chunk_id} · 分数 ${Number.isFinite(result.score) ? result.score.toFixed(3) : "未知"}`, 6,
        result.heading_path?.join(" / "));
      connect(kbNode, docNode, "包含");
      connect(docNode, chunkNode, "包含");
      connect(logNode, chunkNode, "工具返回");
    }
  }

  add(taskNode, `任务 #${task.id} · ${task.name?.trim() ? task.name : task.description.slice(0, 42)}`, 0);
  if (task.parent_ref && /^[1-9]\d*$/.test(task.parent_ref) && task.parent_ref !== task.id) {
    const parentNode = `task:${task.parent_ref}`;
    add(parentNode, `父任务 #${task.parent_ref}`, -1, "编排任务创建时保存的父任务 ID",
      `/function/tasks/detail?id=${encodeURIComponent(task.parent_ref)}`);
    connect(parentNode, taskNode, "派生任务");
  }
  for (const sourceID of task.source_task_ids ?? []) {
    if (!/^[1-9]\d*$/.test(sourceID) || sourceID === task.id) continue;
    const sourceNode = `task:${sourceID}`;
    add(sourceNode, `来源任务 #${sourceID}`, -1, "任务配置中的直接只读继承关系",
      `/function/tasks/detail?id=${encodeURIComponent(sourceID)}`);
    connect(sourceNode, taskNode, "只读继承");
  }
  for (const item of activities) {
    if (item.inherited || item.source_task_id) continue;
    if (item.kind !== "tool_use" && item.kind !== "tool_result") continue;
    const requestID = item.metadata?.llm_request_id;
    const key = `${item.worker}:${item.intent_id ?? ""}:${item.tool_use_id ?? ""}`;
    if (item.kind === "tool_use") {
      const call = `call:${item.seq}`;
      calls.set(key, call);
      add(call, `${item.tool ?? "工具"} · 调用 #${item.seq}`, 2, item.summary);
      if (requestID) {
        const request = `request:${requestID}`;
        add(request, `模型请求 ${requestID.slice(0, 16)}`, 1, requestID,
          `/function/llm-records?task=${encodeURIComponent(task.id)}&request_id=${encodeURIComponent(requestID)}`);
        connect(taskNode, request, "请求 ID");
        connect(request, call, "发起");
      } else {
        connect(taskNode, call, "历史调用");
      }
      continue;
    }
    const call = calls.get(key);
    const source = call ?? `result:${item.seq}`;
    if (!call) {
      add(source, `${item.tool ?? "工具"} · 结果 #${item.seq}`, 2, item.summary);
      connect(taskNode, source, "仅任务归属");
    }
    const ref = item.metadata?.record_ref;
    if (!ref || item.is_error) continue;
    if (ref.kind === "rag_retrieval") {
      addRetrieval(ref.id, source, "返回日志");
    } else if (ref.kind === "finding") {
      const id = `finding:${ref.id}`;
      add(id, `Finding #${ref.id}`, 3, "工具结果中的持久化记录 ID",
        `/function/findings/detail?id=${encodeURIComponent(ref.id)}&context_task=${encodeURIComponent(task.id)}`);
      connect(source, id, "报告");
    } else if (ref.kind === "rag_proposal") {
      const id = `proposal:${ref.id}`;
      add(id, `知识提案 #${ref.id} · 待人工审核`, 3, "提交提案不会直接发布",
        `/system/rag?knowledge_base_id=${encodeURIComponent(ref.knowledge_base_id ?? "")}&tab=proposals#proposal-${encodeURIComponent(ref.id)}`);
      connect(source, id, "提交");
    } else if (ref.kind === "traffic_read") {
      const id = `traffic_read:${ref.id}`;
      add(id, `流量 ${ref.id}`, 3, "traffic_get 成功读取的已有流量；不代表该工具产生此流量",
        `/function/traffic?id=${encodeURIComponent(ref.id)}`);
      connect(source, id, "读取流量");
    }
  }
  for (const log of logs) {
    if (!known.has(`retrieval:${log.id}`)) addRetrieval(String(log.id), taskNode, "仅任务归属");
  }
  for (const finding of findings) {
    if (finding.inherited || !finding.finding_id) continue;
    const id = `finding:${finding.finding_id}`;
    add(id, `Finding #${finding.finding_id} · ${finding.name || finding.vulnclass}`, 3, finding.summary,
      `/function/findings/detail?id=${encodeURIComponent(finding.finding_id)}&context_task=${encodeURIComponent(task.id)}`);
    if (!edges.some((edge) => edge.target === id)) connect(taskNode, id, "仅任务归属");
  }
  for (const item of traffic) {
    const finding = `finding:${item.finding_id}`;
    if (!known.has(finding)) continue;
    for (const binding of item.bindings) {
      const snapshot = `traffic:${binding.snapshot_id}`;
      add(snapshot, `${binding.snapshot.method} ${binding.snapshot.url}`, 4,
        `快照 ${binding.snapshot.id} · 源流量 ${binding.snapshot.source_traffic_id}`,
        `/function/findings/detail?id=${encodeURIComponent(item.finding_id)}&context_task=${encodeURIComponent(task.id)}`);
      connect(finding, snapshot, `证据绑定 #${binding.id}`);
    }
  }
  for (const inclusion of inclusions) {
    const log = logByID.get(String(inclusion.retrieval_log_id));
    if (!log || !log.results.some((result) => String(result.chunk_id) === String(inclusion.chunk_id))) continue;
    const chunk = `chunk:${inclusion.chunk_id}`;
    const request = `request:${inclusion.request_id}`;
    add(request, `模型请求 ${inclusion.request_id.slice(0, 16)}`, 1, inclusion.request_id,
      `/function/llm-records?task=${encodeURIComponent(task.id)}&request_id=${encodeURIComponent(inclusion.request_id)}`);
    connect(taskNode, request, "请求 ID");
    connect(chunk, request, "进入工具结果块");
  }
  return { nodes, edges };
}

export function TaskEvidenceGraph({ task }: { task: Task }) {
  const [activities, setActivities] = React.useState<Activity[]>([]);
  const [logs, setLogs] = React.useState<RAGRetrievalLog[]>([]);
  const [findings, setFindings] = React.useState<Finding[]>([]);
  const [traffic, setTraffic] = React.useState<FindingTraffic[]>([]);
  const [inclusions, setInclusions] = React.useState<RAGPromptInclusion[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState("");
  const [revision, setRevision] = React.useState(0);
  const [selected, setSelected] = React.useState<EvidenceNode["data"] | null>(null);

  // biome-ignore lint/correctness/useExhaustiveDependencies: revision triggers an explicit refresh.
  React.useEffect(() => {
    let active = true;
    setLoading(true);
    Promise.all([
      api.activityHistory(task.id, "all", 0, MAX_ACTIVITIES),
      api.taskRAGRetrievalLogs(task.id, MAX_LOGS),
      api.findings(task.id),
      api.taskRuntimeMetrics(task.id),
    ]).then(async ([page, nextLogs, nextFindings, metrics]) => {
      const nextTraffic = await Promise.all(nextFindings
        .filter((finding) => !finding.inherited && finding.finding_id && (finding.traffic_count ?? 0) > 0)
        .slice(0, 40)
        .map((finding) => api.findingTraffic(finding.finding_id!, task.id)));
      if (!active) return;
      setActivities(page.items);
      setLogs(nextLogs);
      setFindings(nextFindings);
      setTraffic(nextTraffic);
      setInclusions(metrics.prompt_inclusions);
      setError("");
    }).catch((cause: Error) => {
      if (active) setError(cause.message || "证据记录加载失败");
    }).finally(() => {
      if (active) setLoading(false);
    });
    return () => { active = false; };
  }, [task.id, revision]);

  const graph = React.useMemo(() => buildEvidenceGraph(task, activities, logs, findings, traffic, inclusions), [task, activities, logs, findings, traffic, inclusions]);
  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b px-4 py-2 text-xs text-muted-foreground">
        <span>最近 {activities.length} 条活动、{logs.length} 次检索、{findings.length} 项发现；只连接明确记录的 ID，孤立记录仅表示任务归属。</span>
        <Button size="icon-sm" variant="ghost" title="刷新证据图" aria-label="刷新证据图" onClick={() => setRevision((value) => value + 1)}>
          <RefreshCwIcon />
        </Button>
      </div>
      {error && <p role="alert" className="p-4 text-destructive text-sm">{error}</p>}
      {loading ? <p className="p-4 text-muted-foreground text-sm">加载中…</p> : !error && (
        <>
          <div className="min-h-0 flex-1 overflow-auto md:hidden">
            {graph.edges.length === 0 && <p className="p-4 text-muted-foreground text-sm">暂无可关联的活动或检索记录。</p>}
            {graph.edges.slice(0, 80).map((edge) => {
              const source = graph.nodes.find((node) => node.id === edge.source);
              const target = graph.nodes.find((node) => node.id === edge.target);
              return (
                <div key={edge.id} className="border-b px-4 py-3 text-sm">
                  <p className="break-words text-muted-foreground text-xs">{source?.data.label} · {edge.label}</p>
                  {target?.data.href
                    ? <Link className="mt-1 block break-words font-medium underline" href={target.data.href}>{target.data.label}</Link>
                    : <p className="mt-1 break-words font-medium">{target?.data.label}</p>}
                </div>
              );
            })}
            {graph.edges.length > 80 && <p className="p-4 text-muted-foreground text-xs">更多关系请在桌面图谱或原始记录中查看。</p>}
          </div>
          <div className="hidden min-h-0 flex-1 md:block">
            <ReactFlow
              nodes={graph.nodes}
              edges={graph.edges}
              fitView
              nodesDraggable={false}
              nodesConnectable={false}
              onNodeClick={(_, node) => setSelected(node.data)}
              proOptions={{ hideAttribution: true }}
            >
              <Background variant={BackgroundVariant.Dots} gap={24} size={1} />
              <Controls showInteractive={false} />
            </ReactFlow>
          </div>
        </>
      )}
      {selected && (
        <div className="border-t px-4 py-3 text-sm">
          <p className="font-medium">{selected.label}</p>
          {selected.detail && <p className="mt-1 break-all text-muted-foreground text-xs">{selected.detail}</p>}
          {selected.href && <Link className="mt-1 inline-flex items-center gap-1 text-xs underline" href={selected.href}>
            查看原始记录 <ExternalLinkIcon className="size-3" />
          </Link>}
        </div>
      )}
    </div>
  );
}

export function TaskEvidenceSheet({ task, onClose }: { task: Task | null; onClose: () => void }) {
  return (
    <Sheet open={task !== null} onOpenChange={(open) => { if (!open) onClose(); }}>
      {task && (
        <SheetContent className="flex h-full w-full! max-w-none! flex-col gap-0 overflow-hidden p-0 sm:w-[min(92vw,100rem)]! sm:max-w-none!">
          <SheetHeader className="border-b px-4 py-3 pr-12">
            <SheetTitle>任务 #{task.id} · 证据关系</SheetTitle>
          </SheetHeader>
          <div className="min-h-0 flex-1"><TaskEvidenceGraph key={task.id} task={task} /></div>
        </SheetContent>
      )}
    </Sheet>
  );
}
