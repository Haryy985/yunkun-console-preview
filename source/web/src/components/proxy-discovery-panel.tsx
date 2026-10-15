"use client";

import * as React from "react";

import { PencilIcon, PlayIcon, PlusIcon, RefreshCwIcon, SearchIcon, ShieldCheckIcon, Trash2Icon } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { TablePagination } from "@/components/table-pagination";
import { api } from "@/lib/api";
import { proxyVerificationMessage } from "@/lib/proxy-verification";
import type { ProxyDiscoveryResult, ProxyDiscoverySource, ProxyPool } from "@/lib/proxy-pool-types";

const providerLabels = { fofa: "FOFA", quake: "Quake" } as const;

function formatTime(value?: string) {
  return value ? new Date(value).toLocaleString("zh-CN") : "未运行";
}

function hasGlobalScope(scope: string) {
  return scope.split(/[,;\r\n]/).some((value) => ["0.0.0.0", "0.0.0.0/0", "::/0"].includes(value.trim()));
}

function resultLabel(result: ProxyDiscoveryResult) {
  if (result.state === "verified") return "已验证";
  if (result.state === "approved") return "已入池";
  if (result.state === "ignored") return "已忽略";
  if (result.state === "invalid") return "验证失败";
  if (result.state === "verifying") return "验证中";
  return "未验证";
}

export function ProxyDiscoveryPanel({ onCreatePool, onOpenPool, onChanged, refreshKey = 0 }: {
  onCreatePool: () => void;
  onOpenPool: (poolID: number) => void;
  onChanged: () => void;
  refreshKey?: number;
}) {
  const [sources, setSources] = React.useState<ProxyDiscoverySource[]>([]);
  const [results, setResults] = React.useState<ProxyDiscoveryResult[]>([]);
  const [pools, setPools] = React.useState<ProxyPool[]>([]);
  const [selectedSource, setSelectedSource] = React.useState<number | null>(null);
  const [loading, setLoading] = React.useState(true);
  const [busy, setBusy] = React.useState(false);
  const [showForm, setShowForm] = React.useState(false);
  const [targetPool, setTargetPool] = React.useState("");
  const [editing, setEditing] = React.useState<number>();
  const [page, setPage] = React.useState(1);
  const [pageSize, setPageSize] = React.useState(20);
  const [total, setTotal] = React.useState(0);
  const [resultLoading, setResultLoading] = React.useState(false);
  const [resultError, setResultError] = React.useState("");
  const [revision, setRevision] = React.useState(0);
  const [stateFilter, setStateFilter] = React.useState("");
  const [selectedIds, setSelectedIds] = React.useState<Set<number>>(() => new Set());
  const [summary, setSummary] = React.useState({ total: 0, pending: 0, verified: 0, invalid: 0, approved: 0, ignored: 0 });
  const [batchProgress, setBatchProgress] = React.useState<{ done: number; total: number } | null>(null);
  const stopBatch = React.useRef(false);
  const batchLock = React.useRef(false);
  const mounted = React.useRef(true);
  const resultRequest = React.useRef(0);
  const [form, setForm] = React.useState({
    provider: "fofa" as "fofa" | "quake",
    name: "",
    query: "",
    authorized_scope: "",
    verify_url: "",
    fetch_limit: 100,
    email: "",
    key: "",
  });

  React.useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      stopBatch.current = true;
    };
  }, []);

  const load = React.useCallback(async () => {
    setLoading(true);
    try {
      const [sourceResponse, poolResponse] = await Promise.all([
        api.proxyDiscoverySources(),
        api.proxyPools(),
      ]);
      setSources(sourceResponse.sources);
      setPools(poolResponse.pools);
      setTargetPool((current) => poolResponse.pools.some((pool) => String(pool.id) === current)
        ? current : String(poolResponse.pools[0]?.id ?? ""));
      setSelectedSource((current) =>
        current && sourceResponse.sources.some((source) => source.id === current)
          ? current
          : (sourceResponse.sources[0]?.id ?? null),
      );
      setRevision((value) => value + 1);
    } catch (error) {
      toast.error((error as Error).message || "发现数据加载失败");
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => {
    void load();
  }, [load, refreshKey]);

  React.useEffect(() => {
    const request = ++resultRequest.current;
    setResults([]);
    setResultError("");
    if (!selectedSource) {
      setTotal(0);
      setSummary({ total: 0, pending: 0, verified: 0, invalid: 0, approved: 0, ignored: 0 });
      setResultLoading(false);
      return;
    }
    setResultLoading(true);
    void api.proxyDiscoveryResults(selectedSource, page, pageSize, stateFilter).then((response) => {
      if (request !== resultRequest.current) return;
      setResults(response.results);
      setTotal(response.total);
      setSummary(response.summary);
      if (response.page !== page) setPage(response.page);
    }).catch((error) => {
      if (request === resultRequest.current) setResultError((error as Error).message);
    }).finally(() => {
      if (request === resultRequest.current) setResultLoading(false);
    });
    return () => { resultRequest.current++; };
  }, [selectedSource, page, pageSize, revision, stateFilter]);

  React.useEffect(() => {
    setSelectedIds(new Set());
  }, [selectedSource, stateFilter]);

  function editSource(source?: ProxyDiscoverySource) {
    setEditing(source?.id);
    setForm({
      provider: source?.provider ?? "fofa", name: source?.name ?? "", query: source?.query ?? "",
      authorized_scope: source?.authorized_scope ?? "", verify_url: source?.verify_url ?? "",
      fetch_limit: source?.fetch_limit ?? 100, email: "", key: "",
    });
    setShowForm(true);
  }

  async function saveSource(event: React.FormEvent) {
    event.preventDefault();
    if (hasGlobalScope(form.authorized_scope) &&
      !window.confirm("该配置包含全地址范围：0.0.0.0 或 0.0.0.0/0 表示全部 IPv4，::/0 表示全部 IPv6。确认保存？")) return;
    setBusy(true);
    try {
      const credentials: Record<string, string> =
        form.provider === "fofa"
          ? { email: form.email.trim(), key: form.key.trim() }
          : { key: form.key.trim(), email: "" };
      const response = await api.saveProxyDiscoverySource({
        provider: form.provider,
        name: form.name.trim(),
        query: form.query.trim(),
        authorized_scope: form.authorized_scope.trim(),
        verify_url: form.verify_url.trim(),
        fetch_limit: form.fetch_limit,
        ...(editing && !form.key.trim() ? {} : { credentials }),
      }, editing);
      toast.success("发现来源已保存");
      setShowForm(false);
      setSelectedSource(response.id);
      setSelectedIds(new Set());
      setPage(1);
      setForm({
        provider: "fofa",
        name: "",
        query: "",
        authorized_scope: "",
        verify_url: "",
        fetch_limit: 100,
        email: "",
        key: "",
      });
      await load();
      onChanged();
    } catch (error) {
      toast.error((error as Error).message || "保存发现来源失败");
    } finally {
      setBusy(false);
    }
  }

  async function runSource(source: ProxyDiscoverySource) {
    setBusy(true);
    try {
      const response = await api.runProxyDiscovery(source.id);
      toast.success(`已获取 ${response.fetched} 条候选结果`);
      await load();
      setPage(1);
      onChanged();
    } catch (error) {
      toast.error((error as Error).message || "发现查询失败");
    } finally {
      setBusy(false);
    }
  }

  async function verifyResult(result: ProxyDiscoveryResult) {
    setBusy(true);
    try {
      await api.verifyProxyDiscoveryResult(result.id);
      toast.success("候选代理验证完成");
      await load();
      onChanged();
    } catch (error) {
      toast.error(proxyVerificationMessage((error as Error).message || "代理验证失败"));
      await load();
    } finally {
      setBusy(false);
    }
  }

  async function approveResult(result: ProxyDiscoveryResult, target: "candidate" | "production") {
    if (!targetPool) {
      toast.error("请先选择目标代理池");
      return;
    }
    if (target === "production" && !window.confirm(`将 ${result.address} 加入目标代理池并启用？`)) return;
    setBusy(true);
    try {
      await api.approveProxyDiscoveryResult(result.id, Number(targetPool), target);
      toast.success(target === "production" ? "已入池并启用" : "已入池，保持停用", {
        action: { label: "查看代理 IP", onClick: () => onOpenPool(Number(targetPool)) },
      });
      await load();
      onChanged();
    } catch (error) {
      toast.error(proxyVerificationMessage((error as Error).message || "批准代理失败"));
    } finally {
      setBusy(false);
    }
  }

  async function deleteResult(result: ProxyDiscoveryResult) {
    if (!window.confirm(`永久删除候选 ${result.address}？不会删除已入池的代理。`)) return;
    setBusy(true);
    try {
      await api.deleteProxyDiscoveryResult(result.id);
      setSelectedIds((current) => new Set([...current].filter((id) => id !== result.id)));
      toast.success("已删除候选结果");
      await load();
      onChanged();
    } catch (error) {
      toast.error((error as Error).message || "删除失败");
    } finally {
      setBusy(false);
    }
  }

  const selectedSourceItem = sources.find((source) => source.id === selectedSource);
  const pageIDs = results.filter((result) => result.state !== "approved").map((result) => result.id);
  const selectedOnPage = pageIDs.filter((id) => selectedIds.has(id)).length;
  const verifyBlocked = !selectedSourceItem?.enabled || !selectedSourceItem.authorized_scope || !selectedSourceItem.verify_url;

  function admissionReason(result: ProxyDiscoveryResult) {
    if (!selectedSourceItem?.enabled) return "查询来源已停用";
    if (!result.authorized) return "候选不在配置的范围内";
    if (result.state === "approved") return "已入池，请在代理 IP 中管理";
    if (result.state !== "verified") return "需先验证通过";
    if (!targetPool) return "请选择目标代理池";
    return "";
  }

  const selectedOnPageResults = results.filter((result) => selectedIds.has(result.id));
  const eligibleOnPage = selectedOnPageResults.filter((result) => !admissionReason(result)).length;
  const batchAdmissionReason = !selectedIds.size ? "请先勾选候选"
    : !selectedSourceItem?.enabled ? "查询来源已停用"
    : !targetPool ? "请选择目标代理池"
    : selectedOnPageResults.length === selectedIds.size && eligibleOnPage === 0
      ? "所选候选尚未满足入池条件，请检查验证状态和授权范围"
      : "";

  async function runBatch(action: "verify" | "delete" | "approve", target?: "candidate" | "production") {
    const ids = [...selectedIds];
    if (busy || batchLock.current || ids.length === 0) return;
    if (action === "approve" && batchAdmissionReason) {
      toast.error(batchAdmissionReason);
      return;
    }
    if (action === "delete" && !window.confirm(`永久删除勾选的 ${ids.length} 个候选（含其他分页）？验证失败不一定代表代理失效，建议检查失败原因。`)) return;
    if (action === "approve" && target === "production" &&
      !window.confirm(`将勾选的 ${ids.length} 个候选入池并启用？仅验证通过且在当前范围内的候选会成功。`)) return;
    batchLock.current = true;
    setBusy(true);
    stopBatch.current = false;
    setBatchProgress({ done: 0, total: ids.length });
    const succeeded = new Set<number>();
    const failures: string[] = [];
    let done = 0;
    try {
      // Admission can also reverify when the destination pool uses different credentials.
      const chunkSize = action === "delete" ? 100 : 1;
      for (let offset = 0; offset < ids.length && !stopBatch.current; offset += chunkSize) {
        const chunk = ids.slice(offset, offset + chunkSize);
        const response = await api.batchProxyDiscoveryResults(chunk, action, Number(targetPool) || undefined, target);
        for (const item of response.items) {
          if (item.ok) succeeded.add(item.id);
          else failures.push(`#${item.id}: ${proxyVerificationMessage(item.error || "操作失败")}`);
        }
        done += response.items.length;
        if (!mounted.current) return;
        setBatchProgress({ done, total: ids.length });
        setRevision((value) => value + 1);
      }
      if (succeeded.size) toast.success(`成功 ${succeeded.size} 条${done < ids.length ? `，另 ${ids.length - done} 条未执行` : ""}`);
      if (failures.length) toast.error(`失败 ${failures.length} 条，已保留勾选`, { description: failures[0] });
    } catch (error) {
      if (mounted.current) toast.error(`批量操作中断，未确认成功的项保留勾选：${(error as Error).message}`);
    } finally {
      batchLock.current = false;
      if (mounted.current) {
        setSelectedIds((current) => new Set([...current].filter((id) => !succeeded.has(id))));
        setBatchProgress(null);
        await load();
        setBusy(false);
        onChanged();
      }
    }
  }

  return (
    <section className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="font-semibold text-lg">代理发现</h2>
          <p className="mt-1 max-w-2xl text-muted-foreground text-sm">
            发现结果 {total} 条 · 已入池的代理在总览和代理 IP 中管理
          </p>
        </div>
        <div className="flex gap-2">
          <Button
            variant="outline"
            size="icon"
            title="刷新发现数据"
            aria-label="刷新发现数据"
            onClick={() => void load()}
            disabled={loading || busy}
          >
            <RefreshCwIcon className={loading ? "animate-spin" : ""} />
          </Button>
          <Button onClick={() => editSource()} disabled={busy}>
            <PlusIcon data-icon="inline-start" />
            添加查询
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-[280px_minmax(0,1fr)]">
        <div className="min-w-0 space-y-3">
          <div className="flex items-center justify-between">
            <p className="font-medium text-sm">保存的查询</p>
            <Badge variant="outline">{sources.length}</Badge>
          </div>
          {sources.map((source) => (
            <button
              key={source.id}
              type="button"
              onClick={() => {
                if (busy || source.id === selectedSource) return;
                setSelectedSource(source.id);
                setPage(1);
                setTotal(0);
                setSummary({ total: 0, pending: 0, verified: 0, invalid: 0, approved: 0, ignored: 0 });
              }}
              disabled={busy}
              className={`w-full border p-3 text-left ${selectedSource === source.id ? "border-primary bg-primary/5" : "hover:bg-muted/50"}`}
            >
              <div className="flex min-w-0 items-center justify-between gap-2">
                <span className="min-w-0 truncate font-medium text-sm">{source.name}</span>
                <Badge variant="secondary">{providerLabels[source.provider]}</Badge>
              </div>
              <p className="mt-2 truncate text-muted-foreground text-xs">{source.query}</p>
              <p className="mt-2 text-muted-foreground text-xs">上次执行：{formatTime(source.last_run_at)}</p>
              {source.last_error ? <p className="mt-1 truncate text-destructive text-xs">{source.last_error}</p> : null}
            </button>
          ))}
          {!loading && sources.length === 0 ? (
            <p className="border border-dashed p-6 text-center text-muted-foreground text-sm">还没有保存的查询。</p>
          ) : null}
        </div>

        <div className="min-w-0 space-y-4">
          {selectedSourceItem ? (
            <div className="border p-4">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0">
                  <p className="break-all font-medium">{selectedSourceItem.name}</p>
                  <p className="mt-1 break-all text-muted-foreground text-sm">{selectedSourceItem.query}</p>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Button size="sm" variant="outline" disabled={busy} onClick={() => editSource(selectedSourceItem)}>
                    <PencilIcon data-icon="inline-start" />
                    编辑配置
                  </Button>
                  <Button size="sm" disabled={busy} onClick={() => void runSource(selectedSourceItem)}>
                    <PlayIcon data-icon="inline-start" />
                    手动执行
                  </Button>
                  <Button
                    size="icon"
                    variant="ghost"
                    title="删除查询"
                    aria-label={`删除查询 ${selectedSourceItem.name}`}
                    disabled={busy}
                    onClick={async () => {
                      if (!window.confirm(`删除查询「${selectedSourceItem.name}」吗？`)) return;
                      setBusy(true);
                      try {
                        await api.deleteProxyDiscoverySource(selectedSourceItem.id);
                        await load();
                        onChanged();
                      } catch (error) {
                        toast.error((error as Error).message || "删除查询失败");
                      } finally {
                        setBusy(false);
                      }
                    }}
                  >
                    <Trash2Icon className="text-destructive" />
                  </Button>
                </div>
              </div>
              <div className="mt-4 grid gap-3 text-xs sm:grid-cols-4">
                <div>
                  <span className="text-muted-foreground">授权范围</span>
                  <p className="mt-1 break-all">{selectedSourceItem.authorized_scope || "未设置"}</p>
                  {hasGlobalScope(selectedSourceItem.authorized_scope) ? (
                    <p className="mt-1 text-amber-700 dark:text-amber-400">已配置全地址范围。</p>
                  ) : null}
                </div>
                <div>
                  <span className="text-muted-foreground">验证地址</span>
                  <p className="mt-1 break-all">{selectedSourceItem.verify_url || "未设置"}</p>
                </div>
                <div>
                  <span className="text-muted-foreground">每次抓取</span>
                  <p className="mt-1">{selectedSourceItem.fetch_limit} 条</p>
                </div>
                <div>
                  <span className="text-muted-foreground">候选结果</span>
                  <p className="mt-1">{total} 条</p>
                </div>
              </div>
            </div>
          ) : (
            <div className="border border-dashed p-10 text-center text-muted-foreground text-sm">
              选择一个查询开始发现。
            </div>
          )}
          {selectedSourceItem && (!selectedSourceItem.authorized_scope || !selectedSourceItem.verify_url) ? (
            <div role="status" className="flex flex-wrap items-center justify-between gap-3 border-l-2 border-amber-500 bg-muted/30 p-3 text-sm">
              <span>{!selectedSourceItem.authorized_scope ? "待配置代理使用授权范围。" : ""}
                {!selectedSourceItem.verify_url ? "待配置验证地址。" : ""} 候选已保存，尚未执行验证。</span>
              <Button size="sm" variant="outline" onClick={() => editSource(selectedSourceItem)}>补充配置</Button>
            </div>
          ) : null}

          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <p className="font-medium">候选代理</p>
              <p className="mt-1 text-muted-foreground text-xs">授权范围内验证通过的候选，可选择停用入池或入池并启用。</p>
            </div>
            <div className="flex flex-wrap items-center gap-2">
            <select
              className="h-9 w-full min-w-0 max-w-full rounded-md border bg-background px-3 text-sm sm:w-52"
              aria-label="目标代理池"
              value={targetPool}
              disabled={busy}
              onChange={(event) => setTargetPool(event.target.value)}
            >
              <option value="">选择批准目标代理池</option>
              {pools.map((pool) => (
                <option key={pool.id} value={pool.id}>
                  {pool.config.name}
                </option>
              ))}
            </select>
            <Button size="sm" variant="outline" disabled={busy} onClick={onCreatePool}><PlusIcon data-icon="inline-start" />新建代理池</Button>
            {targetPool ? <Button size="sm" variant="ghost" disabled={busy} onClick={() => onOpenPool(Number(targetPool))}>查看代理 IP</Button> : null}
            </div>
          </div>
          {pools.length === 0 ? <p className="text-sm text-amber-700 dark:text-amber-400">尚无目标代理池。可新建空的固定节点池，接收验证通过的候选。</p> : null}
          <div className="flex flex-wrap items-center gap-3 border-y py-3">
            <select aria-label="候选验证状态" className="h-9 rounded-md border bg-background px-3 text-sm"
              value={stateFilter} disabled={busy}
              onChange={(event) => { setStateFilter(event.target.value); setPage(1); }}>
              <option value="">全部（{summary.total}）</option>
              <option value="pending">待验证（{summary.pending}）</option>
              <option value="verified">验证通过（{summary.verified}）</option>
              <option value="invalid">验证失败（{summary.invalid}）</option>
              <option value="approved">已入池（{summary.approved}）</option>
              <option value="ignored">已忽略（{summary.ignored}）</option>
            </select>
            <span className="text-muted-foreground text-xs">已选 {selectedIds.size} 条 · 当前页可入池 {eligibleOnPage} 条</span>
            <Button size="sm" variant="outline" disabled={busy || !selectedIds.size || verifyBlocked}
              onClick={() => void runBatch("verify")}><ShieldCheckIcon /> 批量验证</Button>
            <span title={batchAdmissionReason || "验证通过，保持停用入池"}>
            <Button size="sm" variant="outline" disabled={busy || !!batchAdmissionReason}
              onClick={() => void runBatch("approve", "candidate")}>停用入池</Button>
            </span>
            <span title={batchAdmissionReason || "验证通过，入池并启用"}>
            <Button size="sm" disabled={busy || !!batchAdmissionReason}
              onClick={() => void runBatch("approve", "production")}>入池并启用</Button>
            </span>
            <Button size="sm" variant="destructive" disabled={busy || !selectedIds.size}
              onClick={() => void runBatch("delete")}><Trash2Icon /> 批量删除</Button>
            {selectedIds.size > 0 ? <Button size="sm" variant="ghost" disabled={busy} onClick={() => setSelectedIds(new Set())}>清空勾选</Button> : null}
            {batchProgress ? <div role="status" className="flex items-center gap-2 text-sm">
              <span>已处理 {batchProgress.done}/{batchProgress.total}</span>
              <Button size="sm" variant="outline" onClick={() => { stopBatch.current = true; }}>停止后续操作</Button>
            </div> : null}
          </div>
          {resultError ? <p role="alert" className="text-destructive text-sm">{resultError}</p> : null}
          <div className="overflow-x-auto border">
            <table className="w-full min-w-[900px] text-left text-sm">
              <thead className="border-b bg-muted/30 text-muted-foreground">
                <tr>
                  <th className="w-10 p-3">
                    <Checkbox aria-label="勾选当前页未入池候选" disabled={busy || pageIDs.length === 0}
                      checked={selectedOnPage === 0 ? false : selectedOnPage === pageIDs.length ? true : "indeterminate"}
                      onCheckedChange={(checked) => setSelectedIds((current) => {
                        const next = new Set(current);
                        for (const id of pageIDs) { if (checked === true) next.add(id); else next.delete(id); }
                        return next;
                      })} />
                  </th>
                  <th className="p-3">地址</th>
                  <th className="p-3">来源</th>
                  <th className="p-3">授权</th>
                  <th className="p-3">验证</th>
                  <th className="p-3">验证耗时</th>
                  <th className="p-3">出口</th>
                  <th className="p-3 text-right">操作</th>
                </tr>
              </thead>
              <tbody>
                {results.map((result) => (
                  <tr key={result.id} className="border-b">
                    <td className="p-3">
                      <Checkbox aria-label={`勾选候选 ${result.address}`} disabled={busy || result.state === "approved"}
                        checked={selectedIds.has(result.id)} onCheckedChange={(checked) => setSelectedIds((current) => {
                          const next = new Set(current);
                          if (checked === true) next.add(result.id); else next.delete(result.id);
                          return next;
                        })} />
                    </td>
                    <td className="max-w-64 p-3 font-mono text-xs">
                      <span className="whitespace-nowrap">{result.address}</span>
                      {result.last_error ? <p className="mt-1 break-words font-sans text-destructive">{proxyVerificationMessage(result.last_error)}</p> : null}
                    </td>
                    <td className="max-w-40 truncate p-3 text-xs" title={sources.find((source) => source.id === result.source_id)?.name}>
                      {sources.find((source) => source.id === result.source_id)?.name ?? "-"}
                    </td>
                    <td className="whitespace-nowrap p-3">
                      {result.authorized ? (
                        <Badge variant="secondary">范围内</Badge>
                      ) : (
                        <Badge variant={result.authorization_status === "unconfigured" ? "outline" : "destructive"}>
                          {result.authorization_status === "unconfigured" ? "待配置范围" : "范围外"}
                        </Badge>
                      )}
                    </td>
                    <td className="whitespace-nowrap p-3">
                      <span className={result.state === "verified" ? "text-emerald-700" : "text-muted-foreground"}>
                        {resultLabel(result)}
                      </span>
                    </td>
                    <td className="whitespace-nowrap p-3 text-xs">{result.latency_ms != null ? `${result.latency_ms} ms` : "-"}</td>
                    <td className="whitespace-nowrap p-3">{result.verified_ip || "-"}</td>
                    <td className="p-3">
                      <div className="flex justify-end gap-1">
                        <Button
                          size="icon"
                          variant="ghost"
                          title={!result.authorized ? "请检查授权范围" : !selectedSourceItem?.verify_url ? "请先配置验证地址" : "验证代理"}
                          aria-label={`验证候选 ${result.id}`}
                          disabled={busy || verifyBlocked || !result.authorized || result.state === "approved" || result.state === "ignored"}
                          onClick={() => void verifyResult(result)}
                        >
                          <ShieldCheckIcon />
                        </Button>
                        <span title={admissionReason(result) || "验证通过，保持停用入池"}>
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={busy || !!admissionReason(result)}
                          onClick={() => void approveResult(result, "candidate")}
                        >
                          停用入池
                        </Button>
                        </span>
                        <span title={admissionReason(result) || "验证通过，入池并启用"}>
                        <Button
                          size="sm"
                          disabled={busy || !!admissionReason(result)}
                          onClick={() => void approveResult(result, "production")}
                        >
                          入池并启用
                        </Button>
                        </span>
                        <Button
                          size="icon"
                          variant="ghost"
                          title="删除候选"
                          aria-label={`删除候选 ${result.id}`}
                          disabled={busy || result.state === "approved"}
                          onClick={() => void deleteResult(result)}
                        >
                          <Trash2Icon className="text-muted-foreground" />
                        </Button>
                      </div>
                      {admissionReason(result) ? <p className="mt-1 text-right text-muted-foreground text-xs">{admissionReason(result)}</p> : null}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {resultLoading ? <p className="p-10 text-center text-muted-foreground text-sm">加载中</p> : null}
            {!loading && !resultLoading && !resultError && results.length === 0 ? (
              <p className="p-10 text-center text-muted-foreground text-sm">暂无候选结果。</p>
            ) : null}
          </div>
          {total > 0 ? <TablePagination page={page} pageSize={pageSize} total={total}
            onPageChange={setPage} onPageSizeChange={(size) => { setPageSize(size); setPage(1); }} /> : null}
        </div>
      </div>

      {showForm ? (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/20 p-4"
          role="dialog"
          aria-modal="true"
          onMouseDown={(event) => {
            if (event.target === event.currentTarget) setShowForm(false);
          }}
        >
          <form onSubmit={saveSource} className="max-h-[90vh] w-full max-w-xl space-y-4 overflow-y-auto border bg-background p-5 shadow-xl">
            <div>
              <h3 className="font-semibold">{editing ? "编辑发现查询" : "添加发现查询"}</h3>
              <p className="mt-1 text-muted-foreground text-sm">查询结果只进入候选区，不会自动启用。</p>
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="discovery-provider">平台</Label>
                <select
                  id="discovery-provider"
                  className="h-9 w-full rounded-md border bg-background px-3 text-sm"
                  value={form.provider}
                  disabled={!!editing}
                  onChange={(event) => setForm({ ...form, provider: event.target.value as "fofa" | "quake" })}
                >
                  <option value="fofa">FOFA</option>
                  <option value="quake">Quake</option>
                </select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="discovery-name">名称</Label>
                <Input
                  id="discovery-name"
                  required
                  value={form.name}
                  onChange={(event) => setForm({ ...form, name: event.target.value })}
                />
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="discovery-query">查询语句</Label>
              <Textarea
                id="discovery-query"
                required
                rows={3}
                value={form.query}
                onChange={(event) => setForm({ ...form, query: event.target.value })}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="discovery-fetch-limit">每次抓取数量</Label>
              <Input
                id="discovery-fetch-limit"
                type="number"
                min={1}
                max={100}
                required
                value={form.fetch_limit}
                onChange={(event) => setForm({ ...form, fetch_limit: Number(event.target.value) })}
              />
              <p className="text-muted-foreground text-xs">每次手动执行最多获取 1 到 100 个代理地址。</p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="discovery-scope">授权范围（可选）</Label>
              <Input
                id="discovery-scope"
                placeholder="例如：203.0.113.0/24 或 example.com"
                value={form.authorized_scope}
                onChange={(event) => setForm({ ...form, authorized_scope: event.target.value })}
              />
              <p className="text-muted-foreground text-xs">未设置时，查询结果不会通过授权校验。</p>
              {hasGlobalScope(form.authorized_scope) ? (
                <p className="text-amber-700 text-xs dark:text-amber-400">0.0.0.0 将保存为全部 IPv4 范围 0.0.0.0/0；IPv6 使用 ::/0。仅配置你有权使用的范围。</p>
              ) : null}
            </div>
            <div className="space-y-2">
              <Label htmlFor="discovery-verify-url">代理验证地址（可选）</Label>
              <Input
                id="discovery-verify-url"
                type="url"
                placeholder="https://your-authorized-check.example/ip"
                value={form.verify_url}
                onChange={(event) => setForm({ ...form, verify_url: event.target.value })}
              />
              <p className="text-muted-foreground text-xs">验证地址须返回出口 IP：纯文本，或 JSON 的 ip／origin 字段。未设置时不能验证候选。</p>
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              {form.provider === "fofa" ? (
                <div className="space-y-2">
                  <Label htmlFor="discovery-email">FOFA Email</Label>
                  <Input
                    id="discovery-email"
                    type="email"
                    required={!editing || !!form.key.trim()}
                    value={form.email}
                    onChange={(event) => setForm({ ...form, email: event.target.value })}
                  />
                </div>
              ) : null}
              <div className="space-y-2">
                <Label htmlFor="discovery-key">{form.provider === "fofa" ? "FOFA Key" : "Quake Key"}</Label>
                <Input
                  id="discovery-key"
                  type="password"
                  required={!editing}
                  placeholder={editing ? "留空保留现有凭据" : ""}
                  value={form.key}
                  onChange={(event) => setForm({ ...form, key: event.target.value })}
                />
              </div>
            </div>
            <div className="flex justify-end gap-2">
              <Button type="button" variant="outline" disabled={busy} onClick={() => setShowForm(false)}>
                取消
              </Button>
              <Button type="submit" disabled={busy}>
                <SearchIcon data-icon="inline-start" />
                保存查询
              </Button>
            </div>
          </form>
        </div>
      ) : null}
    </section>
  );
}
