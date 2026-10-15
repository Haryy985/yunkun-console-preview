"use client";

import * as React from "react";

import {
  BookOpenIcon,
  CheckCircle2Icon,
  FileTextIcon,
  FlaskConicalIcon,
  FolderOpenIcon,
  Loader2Icon,
  PlusIcon,
  RefreshCwIcon,
  SearchIcon,
  Trash2Icon,
  UploadIcon,
  XCircleIcon,
} from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { api } from "@/lib/api";
import type {
  RAGCapabilities,
  RAGChangeProposal,
  RAGDocument,
  RAGDocumentDetail,
  RAGIngestionJob,
  RAGKnowledgeBase,
  RAGSearchResult,
  RAGRetrievalLog,
  RAGUploadResult,
} from "@/lib/types";

const statusLabels: Record<RAGDocument["status"], string> = {
  uploaded: "待处理",
  parsing: "解析中",
  parsed: "已解析",
  chunking: "切片中",
  chunked: "已切片",
  embedding: "向量化中",
  ready: "已就绪",
  failed: "失败",
};

const visibilityLabels: Record<RAGKnowledgeBase["visibility"], string> = {
  private: "私有",
  team: "团队",
  shared: "共享",
  public: "公开",
};

const proposalStatusLabels: Record<RAGChangeProposal["status"], string> = {
  pending: "待审核",
  approved: "已批准",
  rejected: "已驳回",
};

function fmtTime(value: string) {
  return new Date(value).toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function jobStatusClass(status: RAGIngestionJob["status"]) {
  if (status === "succeeded") return "bg-emerald-500";
  if (status === "failed") return "bg-destructive";
  return "bg-amber-500";
}

function retrievalModeLabel(mode: string) {
  if (mode === "hybrid_rrf") return "混合检索 · RRF";
  if (mode === "keyword") return "关键词回退";
  if (mode === "vector") return "向量检索";
  return mode;
}

function resultScoreLabel(mode: string) {
  return mode === "hybrid_rrf" ? "融合分数" : "检索得分";
}

function DocStatus({ status }: { status: RAGDocument["status"] }) {
  if (status === "ready") {
    return (
      <Badge variant="secondary" className="gap-1 text-emerald-700">
        <CheckCircle2Icon className="size-3" />
        {statusLabels[status]}
      </Badge>
    );
  }
  if (status === "failed") {
    return (
      <Badge variant="destructive" className="gap-1">
        <XCircleIcon className="size-3" />
        {statusLabels[status]}
      </Badge>
    );
  }
  return (
    <Badge variant="outline" className="gap-1">
      <Loader2Icon className="size-3 animate-spin" />
      {statusLabels[status]}
    </Badge>
  );
}

function Metric({ label, value, hint }: { label: string; value: React.ReactNode; hint: string }) {
  return (
    <div className="border-l pl-4 first:border-l-0 first:pl-0">
      <p className="font-semibold text-2xl tabular-nums">{value}</p>
      <p className="font-medium text-sm">{label}</p>
      <p className="text-muted-foreground text-xs">{hint}</p>
    </div>
  );
}

export default function RAGPage() {
  const [capabilities, setCapabilities] = React.useState<RAGCapabilities | null>(null);
  const [bases, setBases] = React.useState<RAGKnowledgeBase[]>([]);
  const [documents, setDocuments] = React.useState<RAGDocument[]>([]);
  const [proposals, setProposals] = React.useState<RAGChangeProposal[]>([]);
  const [reviewItem, setReviewItem] = React.useState<RAGChangeProposal | null>(null);
  const [reviewOriginal, setReviewOriginal] = React.useState<RAGDocumentDetail | null>(null);
  const [reviewReason, setReviewReason] = React.useState("");
  const [reviewBusy, setReviewBusy] = React.useState(false);
  const [showDeleted, setShowDeleted] = React.useState(false);
  const [jobs, setJobs] = React.useState<RAGIngestionJob[]>([]);
  const [evalSets, setEvalSets] = React.useState<import("@/lib/types").RAGEvalSet[]>([]);
  const [evalCases, setEvalCases] = React.useState<import("@/lib/types").RAGEvalCase[]>([]);
  const [evalRuns, setEvalRuns] = React.useState<import("@/lib/types").RAGEvalRun[]>([]);
  const [selectedEvalSetID, setSelectedEvalSetID] = React.useState<number | null>(null);
  const [evalName, setEvalName] = React.useState("");
  const [evalQuestion, setEvalQuestion] = React.useState("");
  const [evalBusy, setEvalBusy] = React.useState(false);
  const [selectedID, setSelectedID] = React.useState<number | null>(null);
  const [activeTab, setActiveTab] = React.useState("overview");
  const [loading, setLoading] = React.useState(true);
  const [loadError, setLoadError] = React.useState("");
  const [saving, setSaving] = React.useState(false);
  const [creating, setCreating] = React.useState(false);
  const [createOpen, setCreateOpen] = React.useState(false);
  const [createError, setCreateError] = React.useState("");
  const [uploading, setUploading] = React.useState(false);
  const [uploadResult, setUploadResult] = React.useState<RAGUploadResult | null>(null);
  const [query, setQuery] = React.useState("");
  const [searching, setSearching] = React.useState(false);
  const [searchMessage, setSearchMessage] = React.useState("");
  const [searchResults, setSearchResults] = React.useState<RAGSearchResult[]>([]);
  const [searchMeta, setSearchMeta] = React.useState<{
    mode: string;
    latency_ms: number;
    retrieval_log_id: number;
  } | null>(null);
  const [draft, setDraft] = React.useState({
    name: "",
    description: "",
    visibility: "private" as RAGKnowledgeBase["visibility"],
  });
  const [topK, setTopK] = React.useState(5);
  const [retrievalLogs, setRetrievalLogs] = React.useState<RAGRetrievalLog[]>([]);
  const [showContext, setShowContext] = React.useState(false);
  const fileInputRef = React.useRef<HTMLInputElement>(null);
  const folderInputRef = React.useRef<HTMLInputElement>(null);
  const [detail, setDetail] = React.useState<RAGDocumentDetail | null>(null);
  const [detailLoading, setDetailLoading] = React.useState(false);
  const [documentAction, setDocumentAction] = React.useState<number | null>(null);

  const selected = bases.find((item) => item.id === selectedID) ?? bases[0] ?? null;
  const availableEvalSets = evalSets.filter((item) => item.knowledge_base_id === selected?.id);
  const selectedEvalSet = availableEvalSets.find((item) => item.id === selectedEvalSetID) ?? availableEvalSets[0] ?? null;
  const baseScope = React.useRef<{ id: number | undefined }>({ id: selected?.id });
  const documentScope = React.useRef<{ id: number | undefined; showDeleted: boolean }>({ id: selected?.id, showDeleted });
  const evalScope = React.useRef<{ id: number | undefined }>({ id: selectedEvalSet?.id });
  const detailRequest = React.useRef(0);
  const proposalRequest = React.useRef(0);

  // A new scope also invalidates requests when switching A -> B -> A.
  React.useLayoutEffect(() => {
    baseScope.current = { id: selected?.id };
    return () => { baseScope.current = { id: undefined }; };
  }, [selected?.id]);

  React.useLayoutEffect(() => {
    documentScope.current = { id: selected?.id, showDeleted };
    return () => { documentScope.current = { id: undefined, showDeleted: false }; };
  }, [selected?.id, showDeleted]);

  React.useLayoutEffect(() => {
    evalScope.current = { id: selectedEvalSet?.id };
    return () => { evalScope.current = { id: undefined }; };
  }, [selectedEvalSet?.id, selected?.id]);

  React.useEffect(() => {
    if (new URLSearchParams(window.location.search).get("tab") === "proposals") setActiveTab("proposals");
  }, []);

  React.useEffect(() => {
    if (!proposals.length || !window.location.hash.startsWith("#proposal-")) return;
    document.getElementById(window.location.hash.slice(1))?.scrollIntoView({ block: "center" });
  }, [proposals]);

  const loadBases = React.useCallback(async () => {
    setLoading(true);
    try {
      const next = await api.ragKnowledgeBases();
      setBases(next);
      const requested = Number(new URLSearchParams(window.location.search).get("knowledge_base_id"));
      setSelectedID((current) => current ?? next.find((base) => base.id === requested)?.id ?? next[0]?.id ?? null);
      setLoadError("");
    } catch (error) {
      setLoadError((error as Error).message || "知识库加载失败");
    } finally {
      setLoading(false);
    }
  }, []);

  const loadDocuments = React.useCallback(
    async (id: number) => {
      const scope = documentScope.current;
      if (scope.id !== id || scope.showDeleted !== showDeleted) return;
      try {
        const next = await api.ragDocuments(id, showDeleted);
        if (scope === documentScope.current) setDocuments(next);
      } catch (error) {
        if (scope === documentScope.current) toast.error((error as Error).message || "文档加载失败");
      }
    },
    [showDeleted],
  );

  const loadJobs = React.useCallback(async (id: number) => {
    const scope = baseScope.current;
    if (scope.id !== id) return;
    try {
      const next = await api.ragIngestionJobs(id);
      if (scope === baseScope.current) setJobs(next);
    } catch (error) {
      if (scope === baseScope.current) toast.error((error as Error).message || "任务加载失败");
    }
  }, []);

  const loadProposals = React.useCallback(async (id: number) => {
    const scope = baseScope.current;
    if (scope.id !== id) return;
    try {
      const next = await api.ragProposals(id);
      if (scope === baseScope.current) setProposals(next);
    } catch (error) {
      if (scope === baseScope.current) toast.error((error as Error).message || "审核队列加载失败");
    }
  }, []);

  async function openProposal(item: RAGChangeProposal) {
    const request = ++proposalRequest.current;
    setReviewItem(item);
    setReviewReason("");
    setReviewOriginal(null);
    if (item.target_document_id) {
      try {
        const original = await api.ragDocumentDetail(item.target_document_id);
        if (request === proposalRequest.current) setReviewOriginal(original);
      } catch (error) {
        if (request === proposalRequest.current) toast.error((error as Error).message || "原文加载失败");
      }
    }
  }

  async function reviewProposal(decision: "approve" | "reject") {
    if (!reviewItem || reviewBusy) return;
    if (decision === "approve" && !window.confirm("确认已核对来源、事实与敏感信息，并批准发布？")) return;
    setReviewBusy(true);
    try {
      await api.ragReviewProposal(reviewItem.id, decision, reviewReason);
      toast.success(decision === "approve" ? "已批准，正在摄取" : "已驳回");
      setReviewItem(null);
      if (selected) {
        await Promise.all([loadProposals(selected.id), loadDocuments(selected.id), loadJobs(selected.id)]);
      }
    } catch (error) {
      toast.error((error as Error).message || "审核失败");
    } finally {
      setReviewBusy(false);
    }
  }

  const loadEvalSets = React.useCallback(async (id: number) => {
    const scope = baseScope.current;
    if (scope.id !== id) return;
    try {
      const next = await api.ragEvalSets(id);
      if (scope !== baseScope.current) return;
      setEvalSets(next);
      setSelectedEvalSetID((current) => next.some((item) => item.id === current) ? current : next[0]?.id ?? null);
    } catch (error) {
      if (scope === baseScope.current) toast.error((error as Error).message || "评测集加载失败");
    }
  }, []);

  const loadEvalDetails = React.useCallback(async (id: number) => {
    const scope = evalScope.current;
    if (scope.id !== id) return;
    try {
      const [cases, runs] = await Promise.all([api.ragEvalCases(id), api.ragEvalRuns(id)]);
      if (scope !== evalScope.current) return;
      setEvalCases(cases);
      setEvalRuns(runs);
    } catch (error) {
      if (scope === evalScope.current) toast.error((error as Error).message || "评测数据加载失败");
    }
  }, []);

  const loadRetrievalLogs = React.useCallback(async (id: number) => {
    const scope = baseScope.current;
    if (scope.id !== id) return;
    try {
      const next = await api.ragRetrievalLogs(id);
      if (scope === baseScope.current) setRetrievalLogs(next);
    } catch {
      if (scope === baseScope.current) setRetrievalLogs([]);
    }
  }, []);

  React.useEffect(() => {
    void loadBases();
    void api
      .ragCapabilities()
      .then(setCapabilities)
      .catch(() => setCapabilities(null));
  }, [loadBases]);

  React.useEffect(() => {
    setDocuments([]);
    setProposals([]);
    setJobs([]);
    setEvalSets([]);
    setSelectedEvalSetID(null);
    setRetrievalLogs([]);
    setSearchResults([]);
    setSearchMeta(null);
    setSearchMessage("");
    setSearching(false);
    setDetail(null);
    setDetailLoading(false);
    setReviewItem(null);
    setReviewOriginal(null);
    detailRequest.current++;
    proposalRequest.current++;
  }, [selected?.id]);

  React.useEffect(() => {
    const id = selected?.id;
    if (id == null) {
      setDocuments([]);
      setProposals([]);
      setJobs([]);
      return;
    }
    void loadDocuments(id);
    void loadJobs(id);
    void loadProposals(id);
    void loadEvalSets(id);
    void loadRetrievalLogs(id);
    const timer = window.setInterval(() => {
      void loadDocuments(id);
      void loadJobs(id);
      void loadRetrievalLogs(id);
    }, 1800);
    const proposalTimer = window.setInterval(() => void loadProposals(id), 15000);
    return () => {
      window.clearInterval(timer);
      window.clearInterval(proposalTimer);
    };
  }, [selected?.id, loadDocuments, loadJobs, loadProposals, loadEvalSets, loadRetrievalLogs]);

  React.useEffect(() => {
    setEvalCases([]);
    setEvalRuns([]);
    const id = selectedEvalSet?.id;
    if (id != null) void loadEvalDetails(id);
    else {
      setEvalCases([]);
      setEvalRuns([]);
    }
  }, [selectedEvalSet?.id, loadEvalDetails]);

  function openCreateKnowledgeBase() {
    setCreateError("");
    setCreateOpen(true);
  }

  async function createKnowledgeBase() {
    if (creating) return;
    if (!draft.name.trim()) {
      setCreateError("请填写知识库名称");
      return;
    }
    setCreateError("");
    setCreating(true);
    try {
      const item = await api.ragCreateKnowledgeBase({ ...draft, name: draft.name.trim() });
      setBases((items) => [item, ...items]);
      setSelectedID(item.id);
      setActiveTab("overview");
      setCreateOpen(false);
      setDraft({ name: "", description: "", visibility: "private" });
      toast.success("知识库已创建");
    } catch (error) {
      setCreateError((error as Error).message || "创建失败");
    } finally {
      setCreating(false);
    }
  }

  async function toggleEnabled(item: RAGKnowledgeBase, enabled: boolean) {
    setSaving(true);
    try {
      const next = await api.ragUpdateKnowledgeBase(item.id, { enabled });
      setBases((items) => items.map((current) => (current.id === next.id ? next : current)));
      toast.success(enabled ? "知识库已启用" : "知识库已停用");
    } catch (error) {
      toast.error((error as Error).message || "状态更新失败");
    } finally {
      setSaving(false);
    }
  }

  async function deleteSelected() {
    if (!selected || !window.confirm(`确定删除知识库「${selected.name}」吗？`)) return;
    try {
      await api.ragDeleteKnowledgeBase(selected.id);
      const next = bases.filter((item) => item.id !== selected.id);
      setBases(next);
      setSelectedID(next[0]?.id ?? null);
      toast.success("知识库已删除");
    } catch (error) {
      toast.error((error as Error).message || "删除失败");
    }
  }

  async function uploadDocuments(files: File[]) {
    if (!selected || files.length === 0 || uploading) return;
    if (files.length > 500 || files.reduce((total, file) => total + file.size, 0) > 256 * 1024 * 1024) {
      toast.error("每批最多 500 个文件，总大小不能超过 256 MiB");
      return;
    }
    const knowledgeBaseID = selected.id;
    setUploading(true);
    setUploadResult(null);
    try {
      const result = await api.ragUploadDocuments(knowledgeBaseID, files);
      setUploadResult(result);
      await Promise.all([loadBases(), loadDocuments(knowledgeBaseID), loadJobs(knowledgeBaseID)]);
      if (result.uploaded === 0) {
        toast.error("没有导入文档，请查看失败原因");
      } else if (result.skipped > 0) {
        toast.success(`已提交 ${result.uploaded} 个文档，跳过 ${result.skipped} 个`);
      } else {
        toast.success(`已提交 ${result.uploaded} 个文档，正在处理`);
      }
    } catch (error) {
      toast.error((error as Error).message || "文档上传失败");
    } finally {
      setUploading(false);
    }
  }

  async function deleteDocument(document: RAGDocument) {
    if (!window.confirm(`确定删除文档「${document.name}」吗？`)) return;
    try {
      await api.ragDeleteDocument(document.id);
      await Promise.all([loadBases(), loadDocuments(document.knowledge_base_id), loadJobs(document.knowledge_base_id)]);
      toast.success("文档已删除");
    } catch (error) {
      toast.error((error as Error).message || "文档删除失败");
    }
  }

  async function openDocumentDetail(document: RAGDocument) {
    const request = ++detailRequest.current;
    setDetail(null);
    setDetailLoading(true);
    try {
      const next = await api.ragDocumentDetail(document.id);
      if (request === detailRequest.current) setDetail(next);
    } catch (error) {
      if (request === detailRequest.current) toast.error((error as Error).message || "文档详情加载失败");
    } finally {
      if (request === detailRequest.current) setDetailLoading(false);
    }
  }

  async function runDocumentAction(action: "restore" | "reparse" | "retry" | "vector", document: RAGDocument) {
    setDocumentAction(document.id);
    try {
      if (action === "restore") await api.ragRestoreDocument(document.id);
      else if (action === "reparse") await api.ragReparseDocument(document.id);
      else if (action === "retry") await api.ragRetryDocument(document.id);
      else await api.ragRebuildDocumentVector(document.id);
      await Promise.all([loadBases(), loadDocuments(document.knowledge_base_id), loadJobs(document.knowledge_base_id)]);
      toast.success(action === "restore" ? "文档已恢复" : "处理任务已提交");
    } catch (error) {
      toast.error((error as Error).message || "文档操作失败");
    } finally {
      setDocumentAction(null);
    }
  }
  async function createEvalSet() {
    if (!selected || !evalName.trim()) return;
    const scope = baseScope.current;
    setEvalBusy(true);
    try {
      const item = await api.ragCreateEvalSet(selected.id, { name: evalName.trim(), description: "RAG 检索质量基线" });
      if (scope !== baseScope.current) return;
      setEvalSets((items) => [item, ...items]);
      setSelectedEvalSetID(item.id);
      setEvalName("");
      toast.success("评测集已创建");
    } catch (error) {
      toast.error((error as Error).message || "评测集创建失败");
    } finally {
      setEvalBusy(false);
    }
  }

  async function createEvalCase() {
    if (!selectedEvalSet || !evalQuestion.trim()) return;
    const scope = evalScope.current;
    setEvalBusy(true);
    try {
      const item = await api.ragCreateEvalCase(selectedEvalSet.id, { question: evalQuestion.trim() });
      if (scope !== evalScope.current) return;
      setEvalCases((items) => [...items, item]);
      setEvalSets((items) =>
        items.map((set) => (set.id === selectedEvalSet.id ? { ...set, case_count: set.case_count + 1 } : set)),
      );
      setEvalQuestion("");
    } catch (error) {
      toast.error((error as Error).message || "评测问题添加失败");
    } finally {
      setEvalBusy(false);
    }
  }

  async function runEval() {
    if (!selectedEvalSet) return;
    const scope = evalScope.current;
    setEvalBusy(true);
    try {
      const run = await api.ragRunEval(selectedEvalSet.id, 5);
      if (scope !== evalScope.current) return;
      setEvalRuns((items) => [run, ...items]);
      toast.success("评测完成");
    } catch (error) {
      toast.error((error as Error).message || "评测运行失败");
    } finally {
      setEvalBusy(false);
    }
  }

  async function runSearch() {
    if (!query.trim() || !selected) return;
    const scope = baseScope.current;
    setSearching(true);
    setSearchMessage("");
    setSearchResults([]);
    setSearchMeta(null);
    try {
      const response = await api.ragSearch(selected.id, { query, top_k: topK });
      if (scope !== baseScope.current) return;
      setSearchResults(response.results);
      setSearchMeta({
        mode: response.mode,
        latency_ms: response.latency_ms,
        retrieval_log_id: response.retrieval_log_id,
      });
      void loadRetrievalLogs(selected.id);
      if (response.results.length === 0)
        setSearchMessage("没有命中切片。可以尝试更具体的关键词，或先上传并等待文档就绪。");
    } catch (error) {
      if (scope === baseScope.current) setSearchMessage((error as Error).message || "检索失败");
    } finally {
      if (scope === baseScope.current) setSearching(false);
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-4 p-4 md:p-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="flex items-center gap-2">
            <BookOpenIcon className="size-5 text-primary" />
            <h1 className="font-semibold text-xl">RAG 知识库</h1>
          </div>
          <p className="mt-1 text-muted-foreground text-sm">管理攻防知识、文档切片与后续 Agent 检索上下文。</p>
        </div>
        <div className="flex items-center gap-2">
          {capabilities ? (
            <Badge variant="outline" title="当前检索后端能力">
              {capabilities.pgvector ? "pgvector 已探测" : "兼容向量模式"} · RRF
            </Badge>
          ) : null}
          <Button variant="outline" size="sm" onClick={() => void loadBases()} disabled={loading}>
            <RefreshCwIcon className={loading ? "animate-spin" : ""} data-icon="inline-start" />
            刷新
          </Button>
        </div>
      </div>
      {loadError && (
        <div role="alert" className="flex flex-wrap items-center justify-between gap-2 border border-destructive/40 bg-destructive/5 px-3 py-2 text-sm text-destructive">
          <span>知识库加载失败：{loadError}</span>
          <Button size="sm" variant="outline" onClick={() => void loadBases()} disabled={loading}>
            <RefreshCwIcon className={loading ? "animate-spin" : ""} />
            重试
          </Button>
        </div>
      )}
      <div className="grid min-h-0 flex-1 gap-4 lg:grid-cols-[280px_minmax(0,1fr)]">
        <Card className="min-h-0">
          <CardHeader className="flex-row items-center justify-between space-y-0">
            <div>
              <CardTitle className="text-sm">知识库</CardTitle>
              <CardDescription>{bases.length} 个空间</CardDescription>
            </div>
            <Button
              size="icon-sm"
              variant="outline"
              title="新建知识库"
              aria-label="新建知识库"
              onClick={openCreateKnowledgeBase}
            >
              <PlusIcon />
            </Button>
          </CardHeader>
          <CardContent className="space-y-2 overflow-auto">
            {loading ? <p className="text-muted-foreground text-sm">加载中...</p> : null}
            {!loading && !loadError && bases.length === 0 ? <p className="text-muted-foreground text-sm">暂无知识库。</p> : null}
            {bases.map((item) => (
              <button
                key={item.id}
                type="button"
                disabled={uploading}
                onClick={() => { setSelectedID(item.id); setUploadResult(null); }}
                className={`w-full rounded-md border px-3 py-2 text-left transition-colors ${selected?.id === item.id ? "border-primary bg-primary/5" : "hover:bg-muted"}`}
              >
                <div className="flex items-center justify-between gap-2">
                  <span className="truncate font-medium text-sm">{item.name}</span>
                  <span
                    className={`size-2 rounded-full ${item.enabled ? "bg-emerald-500" : "bg-muted-foreground/40"}`}
                  />
                </div>
                <p className="mt-1 truncate text-muted-foreground text-xs">{item.description || "暂无描述"}</p>
                <div className="mt-2 flex gap-2 text-[11px] text-muted-foreground">
                  <span>{item.document_count} 文档</span>
                  <span>{item.chunk_count} 切片</span>
                </div>
              </button>
            ))}
          </CardContent>
        </Card>

        <div className="min-h-0 overflow-auto">
          <Tabs value={activeTab} onValueChange={setActiveTab} className="space-y-4">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <TabsList>
                <TabsTrigger value="overview">总览</TabsTrigger>
                <TabsTrigger value="proposals">
                  待审核{proposals.filter((item) => item.status === "pending").length > 0
                    ? ` (${proposals.filter((item) => item.status === "pending").length})`
                    : ""}
                </TabsTrigger>
                <TabsTrigger value="search">检索测试</TabsTrigger>
                <TabsTrigger value="eval">检索评测</TabsTrigger>
                <TabsTrigger value="jobs">处理任务</TabsTrigger>
              </TabsList>
              {selected ? (
                <div className="flex items-center gap-2">
                  <span className="text-muted-foreground text-xs">{visibilityLabels[selected.visibility]}</span>
                  <Switch
                    checked={selected.enabled}
                    disabled={saving}
                    onCheckedChange={(value) => void toggleEnabled(selected, value)}
                  />
                  <Button size="icon-sm" variant="ghost" title="删除知识库" onClick={() => void deleteSelected()}>
                    <Trash2Icon className="text-destructive" />
                  </Button>
                </div>
              ) : null}
            </div>

            <TabsContent value="overview" className="space-y-4">
              {selected ? (
                <>
                  <Card>
                    <CardHeader>
                      <div className="flex flex-wrap items-start justify-between gap-3">
                        <div>
                          <CardTitle>{selected.name}</CardTitle>
                          <CardDescription>{selected.description || "暂无描述"}</CardDescription>
                        </div>
                        <Badge variant={selected.enabled ? "secondary" : "outline"}>
                          {selected.enabled ? "已启用" : "已停用"}
                        </Badge>
                      </div>
                    </CardHeader>
                    <CardContent className="grid grid-cols-2 gap-4 sm:grid-cols-4">
                      <Metric label="文档" value={selected.document_count} hint="已纳入知识库" />
                      <Metric label="切片" value={selected.chunk_count} hint="等待或已完成处理" />
                      <Metric label="向量化" value={selected.vectorized_chunks} hint="当前模型已生成" />
                      <Metric label="失败" value={selected.failed_documents} hint="需要人工处理" />
                    </CardContent>
                  </Card>
                  <Card>
                    <CardHeader className="flex-row flex-wrap items-center justify-between gap-3 space-y-0">
                      <div>
                        <CardTitle className="text-sm">文档</CardTitle>
                        <CardDescription>PDF、DOCX、XLSX、PPTX、文本及 ZIP；单文档 32 MiB，每批 256 MiB。</CardDescription>
                      </div>
                      <div className="flex items-center gap-2">
                        <span className="text-muted-foreground text-xs">显示已删除</span>
                        <Switch checked={showDeleted} onCheckedChange={setShowDeleted} />
                      </div>
                      <input
                        ref={fileInputRef}
                        type="file"
                        multiple
                        accept=".md,.markdown,.txt,.text,.json,.csv,.html,.htm,.docx,.pdf,.xlsx,.pptx,.zip,.xml,.yaml,.yml,.log,.sql,.ini,.conf,.properties,.rst,.go,.py,.js,.ts,.tsx,.jsx,.sh"
                        className="hidden"
                        onChange={(event) => {
                          const files = Array.from(event.target.files ?? []);
                          event.target.value = "";
                          if (files.length > 0) void uploadDocuments(files);
                        }}
                      />
                      <input
                        ref={folderInputRef}
                        type="file"
                        multiple
                        accept=".md,.markdown,.txt,.text,.json,.csv,.html,.htm,.docx,.pdf,.xlsx,.pptx,.zip,.xml,.yaml,.yml,.log,.sql,.ini,.conf,.properties,.rst,.go,.py,.js,.ts,.tsx,.jsx,.sh"
                        className="hidden"
                        {...({ webkitdirectory: "" } as React.InputHTMLAttributes<HTMLInputElement>)}
                        onChange={(event) => {
                          const files = Array.from(event.target.files ?? []);
                          event.target.value = "";
                          if (files.length > 0) void uploadDocuments(files);
                        }}
                      />
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={uploading}
                        onClick={() => fileInputRef.current?.click()}
                      >
                        {uploading ? (
                          <Loader2Icon className="animate-spin" data-icon="inline-start" />
                        ) : (
                          <UploadIcon data-icon="inline-start" />
                        )}
                        {uploading ? "处理中" : "上传文件"}
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={uploading}
                        onClick={() => folderInputRef.current?.click()}
                      >
                        <FolderOpenIcon data-icon="inline-start" />
                        上传文件夹
                      </Button>
                    </CardHeader>
                    <CardContent>
                      {uploadResult ? (
                        <div className="mb-4 space-y-2 border-b pb-3 text-sm" role="status">
                          <p>已提交 {uploadResult.uploaded} 个文档，未导入 {uploadResult.skipped} 个。</p>
                          {uploadResult.errors.length > 0 ? (
                            <ul className="max-h-48 space-y-1 overflow-auto text-destructive text-xs">
                              {uploadResult.errors.map((issue, index) => (
                                <li key={`${issue.name}-${index}`} className="break-words">
                                  {issue.name}：{issue.error}
                                </li>
                              ))}
                            </ul>
                          ) : null}
                        </div>
                      ) : null}
                      {documents.length === 0 ? (
                        <p className="text-muted-foreground text-sm">暂无文档。</p>
                      ) : (
                        <div className="divide-y">
                          {documents.map((document) => (
                            <div key={document.id} className="flex flex-wrap items-center gap-3 py-3">
                              <FileTextIcon className="size-4 text-muted-foreground" />
                              <div className="min-w-0 flex-1">
                                <p className="truncate font-medium text-sm">{document.name}</p>
                                <p className="truncate text-muted-foreground text-xs">
                                  {document.source_path} · {document.chunk_count} 个切片 ·{" "}
                                  {fmtTime(document.updated_at)}
                                </p>
                                {document.error ? (
                                  <p className="mt-1 text-destructive text-xs">{document.error}</p>
                                ) : null}
                              </div>
                              <DocStatus status={document.status} />
                              {document.deleted_at ? (
                                <Button
                                  size="sm"
                                  variant="outline"
                                  onClick={() => void runDocumentAction("restore", document)}
                                  disabled={documentAction === document.id}
                                >
                                  恢复
                                </Button>
                              ) : null}
                              <Button size="sm" variant="outline" onClick={() => void openDocumentDetail(document)}>
                                查看
                              </Button>
                              {document.status === "failed" ? (
                                <Button
                                  size="icon-sm"
                                  variant="ghost"
                                  title="重试文档处理"
                                  disabled={documentAction === document.id}
                                  onClick={() => void runDocumentAction("retry", document)}
                                >
                                  <RefreshCwIcon className="size-4" />
                                </Button>
                              ) : null}
                              {document.status === "ready" ? (
                                <Button
                                  size="icon-sm"
                                  variant="ghost"
                                  title="重建向量"
                                  disabled={documentAction === document.id}
                                  onClick={() => void runDocumentAction("vector", document)}
                                >
                                  <FlaskConicalIcon className="size-4" />
                                </Button>
                              ) : null}
                              <Button
                                size="icon-sm"
                                variant="ghost"
                                title="删除文档"
                                onClick={() => void deleteDocument(document)}
                              >
                                <Trash2Icon className="size-4 text-muted-foreground" />
                              </Button>
                            </div>
                          ))}
                        </div>
                      )}
                    </CardContent>
                  </Card>
                </>
              ) : (
                <Card>
                  <CardContent className="flex min-h-64 flex-col items-center justify-center gap-3 text-muted-foreground text-sm">
                    <p>请选择或创建知识库。</p>
                    <Button onClick={openCreateKnowledgeBase}>
                      <PlusIcon data-icon="inline-start" />
                      新建知识库
                    </Button>
                  </CardContent>
                </Card>
              )}
            </TabsContent>

            <TabsContent value="proposals">
              <section className="space-y-3">
                <div className="flex items-center justify-between">
                  <h2 className="font-medium text-sm">Agent 知识提案</h2>
                  {selected ? (
                    <Button variant="ghost" size="icon-sm" title="刷新审核队列" onClick={() => void loadProposals(selected.id)}>
                      <RefreshCwIcon />
                    </Button>
                  ) : null}
                </div>
                {proposals.length === 0 ? (
                  <p className="text-muted-foreground text-sm">暂无知识提案。</p>
                ) : (
                  <div className="divide-y border-y">
                    {proposals.map((item) => (
                      <div key={item.id} id={`proposal-${item.id}`} className="flex scroll-mt-20 flex-wrap items-center gap-3 py-3 text-sm">
                        <Badge variant={item.status === "pending" ? "outline" : "secondary"}>
                          {proposalStatusLabels[item.status]}
                        </Badge>
                        <div className="min-w-0 flex-1">
                          <p className="truncate font-medium">{item.title}</p>
                          <p className="truncate text-muted-foreground text-xs">
                            {item.kind === "create" ? "新增" : `修订文档 #${item.target_document_id} v${item.base_version}`}
                            {" · "}任务 #{item.task_id} · {item.agent_id} · {fmtTime(item.created_at)}
                          </p>
                        </div>
                        <Button variant="outline" size="sm" onClick={() => void openProposal(item)}>查看</Button>
                      </div>
                    ))}
                  </div>
                )}
              </section>
            </TabsContent>

            <TabsContent value="search">
              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center gap-2">
                    <FlaskConicalIcon className="size-4" />
                    检索测试
                  </CardTitle>
                  <CardDescription>验证关键词、向量与 RRF 融合结果，并查看可追溯的检索记录。</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="flex flex-wrap gap-2">
                    <Input
                      className="min-w-0 flex-1"
                      value={query}
                      onChange={(event) => setQuery(event.target.value)}
                      onKeyDown={(event) => {
                        if (event.key === "Enter") void runSearch();
                      }}
                      placeholder="例如：如何验证 API 越权？"
                    />
                    <select
                      aria-label="返回数量"
                      value={topK}
                      onChange={(event) => setTopK(Number(event.target.value))}
                      className="h-9 rounded-md border bg-background px-2 text-sm"
                    >
                      {[3, 5, 10, 20].map((value) => (
                        <option key={value} value={value}>
                          Top-{value}
                        </option>
                      ))}
                    </select>
                    <Button
                      variant="outline"
                      onClick={() => setShowContext((value) => !value)}
                      disabled={searchResults.length === 0}
                    >
                      {showContext ? "隐藏上下文" : "上下文"}
                    </Button>
                    <Button onClick={() => void runSearch()} disabled={searching || !selected}>
                      <SearchIcon data-icon="inline-start" />
                      检索
                    </Button>
                  </div>
                  {searching ? <p className="text-muted-foreground text-sm">正在准备检索...</p> : null}
                  {searchMessage ? (
                    <div className="rounded-md border border-dashed p-4 text-muted-foreground text-sm">
                      {searchMessage}
                    </div>
                  ) : null}
                  {searchMeta ? (
                    <p className="text-muted-foreground text-xs">
                      模式：{retrievalModeLabel(searchMeta.mode)} · {searchMeta.latency_ms} ms · 检索记录 #
                      {searchMeta.retrieval_log_id}
                    </p>
                  ) : null}
                  {showContext && searchResults.length > 0 ? (
                    <div className="rounded-md border bg-muted/20 p-3">
                      <p className="font-medium text-sm">Agent 上下文预览</p>
                      <pre className="mt-2 max-h-64 overflow-auto whitespace-pre-wrap rounded-md border bg-background p-3 text-xs leading-5">
                        {searchResults
                          .map(
                            (result, index) => `[${index + 1}] ${result.document_name}
${result.content}`,
                          )
                          .join("\n\n")}
                      </pre>
                    </div>
                  ) : null}
                  {searchResults.length > 0 ? (
                    <div className="space-y-3">
                      {searchResults.map((result, index) => (
                        <div key={result.chunk_id} className="rounded-md border p-4">
                          <div className="flex flex-wrap items-center gap-2 text-sm">
                            <Badge variant="secondary">#{index + 1}</Badge>
                            <span className="font-medium">{result.document_name}</span>
                            <span className="text-muted-foreground text-xs">
                              {resultScoreLabel(searchMeta?.mode ?? "")} {result.score.toFixed(4)}
                            </span>
                            {result.heading ? (
                              <span className="text-muted-foreground text-xs">· {result.heading}</span>
                            ) : null}
                          </div>
                          <p className="mt-2 whitespace-pre-wrap text-sm leading-6">{result.content}</p>
                          <p className="mt-2 text-muted-foreground text-xs">
                            chunk #{result.chunk_id} · 行 {result.line_from}-{result.line_to}
                          </p>
                          <p className="text-muted-foreground text-xs">
                            {result.source_uri || "来源未记录"}
                            {result.page_from > 0
                              ? ` · 页 ${result.page_from}-${result.page_to || result.page_from}`
                              : ""}
                            {result.heading_path?.length ? ` · ${result.heading_path.join(" / ")}` : ""}
                          </p>
                        </div>
                      ))}
                    </div>
                  ) : null}
                  {retrievalLogs.length > 0 ? (
                    <div className="border-t pt-4">
                      <div className="mb-2 flex items-center justify-between gap-2">
                        <p className="font-medium text-sm">最近检索记录</p>
                        <span className="text-muted-foreground text-xs">保留最近 {retrievalLogs.length} 条</span>
                      </div>
                      <div className="divide-y rounded-md border">
                        {retrievalLogs.map((log) => (
                          <div key={log.id} className="flex flex-wrap items-center gap-2 px-3 py-2 text-xs">
                            <span className="font-medium">#{log.id}</span>
                            <span className="min-w-40 flex-1 truncate">{log.query}</span>
                            <span className="text-muted-foreground">
                              {retrievalModeLabel(log.mode)} · Top-{log.top_k} · {log.result_count} 命中 ·{" "}
                              {log.latency_ms} ms · {fmtTime(log.created_at)}
                            </span>
                          </div>
                        ))}
                      </div>
                    </div>
                  ) : null}
                  {!searchMessage && !searching && searchResults.length === 0 ? (
                    <p className="text-muted-foreground text-sm">检索结果会显示命中文档、切片、相似度和来源位置。</p>
                  ) : null}
                </CardContent>
              </Card>
            </TabsContent>
            <TabsContent value="jobs">
              <Card>
                <CardHeader>
                  <CardTitle>摄取任务</CardTitle>
                  <CardDescription>解析、切片、Embedding 和索引构建的异步任务状态。</CardDescription>
                </CardHeader>
                <CardContent>
                  {jobs.length === 0 ? (
                    <div className="flex min-h-48 items-center justify-center text-muted-foreground text-sm">
                      当前暂无摄取任务。
                    </div>
                  ) : (
                    <div className="divide-y">
                      {jobs.map((job) => (
                        <div key={job.id} className="flex flex-wrap items-center gap-3 py-3 text-sm">
                          <span className={`size-2 rounded-full ${jobStatusClass(job.status)}`} />
                          <span className="font-medium">{job.kind === "document_ingest" ? "文档摄取" : job.kind}</span>
                          <span className="text-muted-foreground">#{job.document_id ?? "-"}</span>
                          <span className="ml-auto text-muted-foreground text-xs">
                            {job.status} · 尝试 {job.attempts}
                          </span>
                          {job.error ? <span className="basis-full text-destructive text-xs">{job.error}</span> : null}
                        </div>
                      ))}
                    </div>
                  )}
                </CardContent>
              </Card>
            </TabsContent>
            <TabsContent value="eval">
              <div className="grid gap-4 xl:grid-cols-[280px_minmax(0,1fr)]">
                <Card>
                  <CardHeader>
                    <CardTitle className="text-sm">评测集</CardTitle>
                    <CardDescription>用固定问题持续比较检索质量。</CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-3">
                    <div className="flex gap-2">
                      <Input
                        value={evalName}
                        onChange={(event) => setEvalName(event.target.value)}
                        placeholder="例如：API 越权基线"
                      />
                      <Button
                        size="icon"
                        title="创建评测集"
                        disabled={evalBusy || !evalName.trim()}
                        onClick={() => void createEvalSet()}
                      >
                        <PlusIcon />
                      </Button>
                    </div>
                    <div className="space-y-1">
                      {evalSets.map((item) => (
                        <button
                          key={item.id}
                          type="button"
                          onClick={() => setSelectedEvalSetID(item.id)}
                          className={`w-full rounded-md border px-3 py-2 text-left ${selectedEvalSet?.id === item.id ? "border-primary bg-primary/5" : "hover:bg-muted"}`}
                        >
                          <span className="block truncate font-medium text-sm">{item.name}</span>
                          <span className="text-muted-foreground text-xs">{item.case_count} 个问题</span>
                        </button>
                      ))}
                      {evalSets.length === 0 ? <p className="text-muted-foreground text-sm">还没有评测集。</p> : null}
                    </div>
                  </CardContent>
                </Card>
                <Card>
                  <CardHeader className="flex-row items-center justify-between space-y-0">
                    <div>
                      <CardTitle className="text-sm">{selectedEvalSet?.name ?? "检索质量评测"}</CardTitle>
                      <CardDescription>建议先录入 20～50 个带正确引用的问题，再运行基线。</CardDescription>
                    </div>
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={evalBusy || !selectedEvalSet || evalCases.length === 0}
                      onClick={() => void runEval()}
                    >
                      运行评测
                    </Button>
                  </CardHeader>
                  <CardContent className="space-y-4">
                    {selectedEvalSet ? (
                      <>
                        <div className="flex gap-2">
                          <Input
                            value={evalQuestion}
                            onChange={(event) => setEvalQuestion(event.target.value)}
                            placeholder="输入一个要验证的检索问题"
                          />
                          <Button
                            size="sm"
                            disabled={evalBusy || !evalQuestion.trim()}
                            onClick={() => void createEvalCase()}
                          >
                            添加问题
                          </Button>
                        </div>
                        <div className="space-y-2">
                          {evalCases.map((item, index) => (
                            <div key={item.id} className="rounded-md border px-3 py-2 text-sm">
                              <span className="mr-2 text-muted-foreground">#{index + 1}</span>
                              {item.question}
                            </div>
                          ))}
                          {evalCases.length === 0 ? (
                            <p className="text-muted-foreground text-sm">先添加至少一个问题。</p>
                          ) : null}
                        </div>
                        {evalRuns.length > 0 ? (
                          <div className="border-t pt-4">
                            <p className="mb-2 font-medium text-sm">最近一次结果</p>
                            <div className="grid grid-cols-2 gap-3 sm:grid-cols-5">
                              <Metric
                                label="Recall@K"
                                value={`${((evalRuns[0].recall_at_k ?? 0) * 100).toFixed(0)}%`}
                                hint="命中期望引用"
                              />
                              <Metric label="MRR" value={(evalRuns[0].mrr ?? 0).toFixed(2)} hint="首个正确结果排名" />
                              <Metric
                                label="引用命中"
                                value={`${((evalRuns[0].citation_hit_rate ?? 0) * 100).toFixed(0)}%`}
                                hint="可引用结果"
                              />
                              <Metric label="P95" value={`${evalRuns[0].p95_latency_ms ?? 0} ms`} hint="检索耗时" />
                              <Metric
                                label="无答案率"
                                value={`${((evalRuns[0].no_answer_rate ?? 0) * 100).toFixed(0)}%`}
                                hint="拒答样本"
                              />
                            </div>
                          </div>
                        ) : null}
                      </>
                    ) : (
                      <div className="flex min-h-48 items-center justify-center text-muted-foreground text-sm">
                        先创建一个评测集。
                      </div>
                    )}
                  </CardContent>
                </Card>
              </div>
            </TabsContent>
          </Tabs>
        </div>
      </div>
      <Dialog open={createOpen} onOpenChange={(open) => !creating && setCreateOpen(open)}>
        <DialogContent className="sm:max-w-lg" showCloseButton={!creating}>
          <DialogHeader>
            <DialogTitle>新建知识库</DialogTitle>
            <DialogDescription>填写知识库名称、描述与可见性。</DialogDescription>
          </DialogHeader>
          <form
            className="space-y-4"
            onSubmit={(event) => {
              event.preventDefault();
              void createKnowledgeBase();
            }}
          >
            <div className="space-y-2">
              <Label htmlFor="rag-name">名称</Label>
              <Input
                id="rag-name"
                value={draft.name}
                onChange={(event) => setDraft({ ...draft, name: event.target.value })}
                placeholder="例如：API 攻防知识库"
                disabled={creating}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="rag-description">描述</Label>
              <Input
                id="rag-description"
                value={draft.description}
                onChange={(event) => setDraft({ ...draft, description: event.target.value })}
                placeholder="用途和适用范围"
                disabled={creating}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="rag-visibility">可见性</Label>
              <select
                id="rag-visibility"
                className="h-8 w-full rounded-md border bg-background px-2 text-sm"
                value={draft.visibility}
                onChange={(event) =>
                  setDraft({ ...draft, visibility: event.target.value as RAGKnowledgeBase["visibility"] })
                }
                disabled={creating}
              >
                {Object.entries(visibilityLabels).map(([value, label]) => (
                  <option key={value} value={value}>{label}</option>
                ))}
              </select>
            </div>
            {createError ? <p role="alert" className="text-destructive text-sm">{createError}</p> : null}
            <div className="flex justify-end gap-2">
              <Button type="button" variant="outline" disabled={creating} onClick={() => setCreateOpen(false)}>
                取消
              </Button>
              <Button type="submit" disabled={creating}>
                {creating ? <Loader2Icon className="animate-spin" /> : <PlusIcon />}
                {creating ? "创建中..." : "创建"}
              </Button>
            </div>
          </form>
        </DialogContent>
      </Dialog>
      <Dialog open={reviewItem !== null} onOpenChange={(open) => {
        if (!open) { proposalRequest.current++; setReviewItem(null); setReviewOriginal(null); }
      }}>
        <DialogContent className="max-h-[90vh] max-w-5xl overflow-auto">
          <DialogHeader>
            <DialogTitle>{reviewItem?.title ?? "知识提案"}</DialogTitle>
            <DialogDescription>
              {reviewItem?.kind === "update" ? `修订文档 #${reviewItem.target_document_id} v${reviewItem.base_version}` : "新增文档"}
              {" · "}任务 #{reviewItem?.task_id} · {reviewItem?.agent_id}
            </DialogDescription>
          </DialogHeader>
          {reviewItem ? (
            <div className="space-y-4 text-sm">
              <p><strong>来源：</strong>{reviewItem.source_ref}</p>
              <p><strong>理由：</strong>{reviewItem.rationale}</p>
              <div className="grid gap-3 md:grid-cols-2">
                {reviewItem.kind === "update" ? (
                  <div>
                    <p className="mb-1 font-medium">现有版本{reviewOriginal?.content_truncated ? "（预览已截断）" : ""}</p>
                    <pre className="max-h-80 overflow-auto whitespace-pre-wrap border bg-muted/20 p-3 text-xs">
                      {reviewOriginal?.content_preview ?? "正在加载原文"}
                    </pre>
                  </div>
                ) : null}
                <div>
                  <p className="mb-1 font-medium">拟发布内容</p>
                  <pre className="max-h-80 overflow-auto whitespace-pre-wrap border bg-muted/20 p-3 text-xs">{reviewItem.content}</pre>
                </div>
              </div>
              {reviewItem.status === "pending" ? (
                <div className="space-y-2 border-t pt-3">
                  <Label htmlFor="rag-review-reason">审核说明（驳回必填）</Label>
                  <Input id="rag-review-reason" value={reviewReason} onChange={(event) => setReviewReason(event.target.value)} />
                  <div className="flex justify-end gap-2">
                    <Button variant="outline" disabled={reviewBusy || !reviewReason.trim()} onClick={() => void reviewProposal("reject")}>驳回</Button>
                    <Button disabled={reviewBusy || (reviewItem.kind === "update" && (!reviewOriginal || reviewOriginal.content_truncated))} onClick={() => void reviewProposal("approve")}>批准发布</Button>
                  </div>
                </div>
              ) : (
                <p className="text-muted-foreground text-xs">
                  {reviewItem.reviewer} · {reviewItem.reviewed_at ? fmtTime(reviewItem.reviewed_at) : ""}
                  {reviewItem.review_reason ? ` · ${reviewItem.review_reason}` : ""}
                  {reviewItem.published_document_id ? ` · 已发布文档 #${reviewItem.published_document_id}` : ""}
                </p>
              )}
            </div>
          ) : null}
        </DialogContent>
      </Dialog>
      <Dialog open={detail !== null || detailLoading} onOpenChange={(open) => {
        if (!open) { detailRequest.current++; setDetail(null); setDetailLoading(false); }
      }}>
        <DialogContent className="max-w-5xl">
          <DialogHeader>
            <DialogTitle>{detail?.document.name ?? "文档详情"}</DialogTitle>
            <DialogDescription>原文预览、处理状态和切片来源。</DialogDescription>
          </DialogHeader>
          {detailLoading ? (
            <p className="text-muted-foreground text-sm">加载中...</p>
          ) : detail ? (
            <div className="grid min-h-0 gap-4 lg:grid-cols-2">
              <div className="min-h-0 space-y-2">
                <p className="font-medium text-sm">原文预览{detail.content_truncated ? "（已截断）" : ""}</p>
                <pre className="max-h-[55vh] overflow-auto whitespace-pre-wrap rounded-md border bg-muted/30 p-3 text-xs leading-5">
                  {detail.content_preview || "暂无原文"}
                </pre>
              </div>
              <div className="min-h-0 space-y-2">
                <p className="font-medium text-sm">切片 · {detail.chunks.length}</p>
                <div className="max-h-[55vh] space-y-2 overflow-auto">
                  {detail.chunks.map((chunk) => (
                    <div key={chunk.id} className="rounded-md border p-3">
                      <div className="flex items-center justify-between gap-2 text-xs">
                        <span className="font-medium">
                          #{chunk.ordinal} {chunk.heading || "无标题"}
                        </span>
                        <span className="text-muted-foreground">
                          行 {chunk.line_from}-{chunk.line_to}
                        </span>
                      </div>
                      <p className="mt-2 whitespace-pre-wrap text-xs leading-5">{chunk.content}</p>
                    </div>
                  ))}
                  {detail.chunks.length === 0 ? <p className="text-muted-foreground text-sm">暂无切片</p> : null}
                </div>
              </div>
            </div>
          ) : null}
        </DialogContent>
      </Dialog>
    </div>
  );
}
