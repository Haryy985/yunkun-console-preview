"use client";

import * as React from "react";
import {
  ActivityIcon,
  CheckCircle2Icon,
  DownloadIcon,
  GaugeIcon,
  PencilIcon,
  PlusIcon,
  RefreshCwIcon,
  SearchIcon,
  ShieldCheckIcon,
  Trash2Icon,
  WifiOffIcon,
  type LucideIcon,
} from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Switch } from "@/components/ui/switch";
import { Checkbox } from "@/components/ui/checkbox";
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetDescription } from "@/components/ui/sheet";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogCancel,
  AlertDialogAction,
} from "@/components/ui/alert-dialog";
import { ProxyEventsTable } from "@/components/task-network-panel";
import { ProxyDiscoveryPanel } from "@/components/proxy-discovery-panel";
import { TablePagination } from "@/components/table-pagination";
import { useCurrentUser } from "@/hooks/use-current-user";
import { api } from "@/lib/api";
import { proxyVerificationMessage } from "@/lib/proxy-verification";
import type {
  ProxyPool,
  ProxyPoolConfig,
  ProxySecrets,
  ProxyNode,
  ProxyNodeSummary,
  ProxyEvent,
  ProxyGlobalSettings,
} from "@/lib/proxy-pool-types";

const initial: ProxyPoolConfig = {
  name: "",
  kind: "fixed",
  enabled: true,
  protocol: "http",
  addresses: [],
  ttl_seconds: 120,
  fetch_count: 1,
  safety_seconds: 15,
  quota_supported: false,
  quota_key_style: "app",
  low_quota: 100,
  api_url: "",
  items_path: "data",
  host_field: "ip",
  port_field: "port",
  verify_url: "",
  expiry_warning_seconds: 3600,
};
const kinds = { fixed: "固定节点", xiaoxiang: "小象代理", generic: "通用 JSON API" };
const selectClass = "h-9 w-full rounded-md border bg-background px-3 text-sm";
function when(value?: string) {
  return value ? new Date(value).toLocaleString() : "-";
}

function nodeStatus(node: ProxyNode, warningSeconds: number) {
  if (!node.enabled || node.deleted_at) return { label: "已停用", className: "text-muted-foreground" };
  if (
    node.expires_at &&
    Date.parse(node.expires_at) > Date.now() &&
    Date.parse(node.expires_at) <= Date.now() + warningSeconds * 1000
  ) {
    return { label: "即将过期", className: "text-amber-700 dark:text-amber-400" };
  }
  if (node.expires_at && Date.parse(node.expires_at) <= Date.now())
    return { label: "异常", className: "text-destructive" };
  if (node.last_error) return { label: "异常", className: "text-destructive" };
  if (!node.last_success_at) return { label: "待验证", className: "text-muted-foreground" };
  return { label: "可用", className: "text-emerald-700 dark:text-emerald-400" };
}

export default function ProxyPoolsPage() {
  const user = useCurrentUser();
  const admin = user.role === "admin";
  const [pools, setPools] = React.useState<ProxyPool[]>([]);
  const [selected, setSelected] = React.useState<number | null>(null);
  const [nodes, setNodes] = React.useState<ProxyNode[]>([]);
  const [summary, setSummary] = React.useState<ProxyNodeSummary>({
    total: 0,
    available: 0,
    pending: 0,
    expiring: 0,
    error: 0,
    disabled: 0,
  });
  const [events, setEvents] = React.useState<ProxyEvent[]>([]);
  const [error, setError] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const [loading, setLoading] = React.useState(true);
  const [open, setOpen] = React.useState(false);
  const [editing, setEditing] = React.useState<number>();
  const [config, setConfig] = React.useState<ProxyPoolConfig>(initial);
  const [addresses, setAddresses] = React.useState("");
  const [secrets, setSecrets] = React.useState<ProxySecrets>({});
  const [replaceSecrets, setReplaceSecrets] = React.useState(false);
  const [deleting, setDeleting] = React.useState<ProxyPool | null>(null);
  const [nodeQuery, setNodeQuery] = React.useState("");
  const [selectedNodeIDs, setSelectedNodeIDs] = React.useState<number[]>([]);
  const [mainView, setMainView] = React.useState("overview");
  const [discoverySummary, setDiscoverySummary] = React.useState<{
    total: number;
    unapproved: number;
    approved: number;
  } | null>(null);
  const [discoveryRefresh, setDiscoveryRefresh] = React.useState(0);
  const [nodePage, setNodePage] = React.useState(1);
  const [nodePageSize, setNodePageSize] = React.useState(20);
  const [globalSettings, setGlobalSettings] = React.useState<ProxyGlobalSettings>({ enabled: true });
  const detailRequest = React.useRef(0);
  const current = pools.find((p) => p.id === selected);

  const load = React.useCallback(async () => {
    setLoading(true);
    try {
      const r = await api.proxyPools();
      setPools(r.pools);
      setSelected((id) => (r.pools.some((p) => p.id === id) ? id : (r.pools[0]?.id ?? null)));
      setError("");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);
  const loadDetail = React.useCallback(async (id: number) => {
    const request = ++detailRequest.current;
    const results = await Promise.allSettled([api.proxyNodes(id), api.proxyEvents(id)]);
    if (request !== detailRequest.current) return;
    const [n, e] = results;
    if (n.status === "fulfilled") {
      setNodes(n.value.nodes);
      setSummary(n.value.summary);
    } else {
      setNodes([]);
      toast.error(n.reason.message);
    }
    if (e.status === "fulfilled") setEvents(e.value.events);
    else {
      setEvents([]);
      toast.error(e.reason.message);
    }
  }, []);
  React.useEffect(() => {
    void load();
    void api
      .proxySettings()
      .then(setGlobalSettings)
      .catch(() => undefined);
  }, [load]);
  const loadDiscoverySummary = React.useCallback(async () => {
    try {
      const response = await api.proxyDiscoveryResults(undefined, 1, 1);
      setDiscoverySummary(response.summary);
    } catch {
      setDiscoverySummary(null);
    }
  }, []);
  React.useEffect(() => {
    if (admin) void loadDiscoverySummary();
  }, [admin, loadDiscoverySummary]);

  async function updateGlobalSettings(input: Partial<ProxyGlobalSettings>) {
    try {
      const next = await api.updateProxySettings(input);
      setGlobalSettings(next);
      toast.success("代理全局设置已更新");
    } catch (e) {
      toast.error((e as Error).message || "代理全局设置更新失败");
    }
  }
  React.useEffect(() => {
    setNodes([]);
    setSelectedNodeIDs([]);
    setNodePage(1);
    setSummary({ total: 0, available: 0, pending: 0, expiring: 0, error: 0, disabled: 0 });
    setEvents([]);
    if (selected !== null && admin) void loadDetail(selected);
    return () => {
      detailRequest.current++;
    };
  }, [selected, admin, loadDetail]);

  function edit(p?: ProxyPool) {
    setEditing(p?.id);
    setConfig(p ? { ...initial, ...p.config } : { ...initial });
    setAddresses(p?.config.addresses?.join("\n") ?? "");
    setSecrets({});
    setReplaceSecrets(!p);
    setOpen(true);
  }
  async function action(run: () => Promise<unknown>, success: string) {
    setBusy(true);
    try {
      await run();
      toast.success(success);
    } catch (e) {
      toast.error(proxyVerificationMessage((e as Error).message));
    } finally {
      await load();
      if (selected !== null) await loadDetail(selected);
      setBusy(false);
    }
  }
  async function save(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      const c = {
        ...config,
        addresses: addresses
          .split(/\r?\n/)
          .map((s) => s.trim())
          .filter(Boolean),
      };
      const result = await api.saveProxyPool(c, replaceSecrets ? secrets : undefined, editing);
      setOpen(false);
      setSecrets({});
      await load();
      setSelected(result.id);
      setDiscoveryRefresh((value) => value + 1);
      toast.success("代理来源已保存");
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  function field<K extends keyof ProxyPoolConfig>(key: K, value: ProxyPoolConfig[K]) {
    setConfig((c) => ({ ...c, [key]: value }));
  }
  const secretField = (key: keyof ProxySecrets, label: string, password = false) => (
    <div className="space-y-2" key={key}>
      <Label htmlFor={`secret-${key}`}>{label}</Label>
      <Input
        id={`secret-${key}`}
        type={password ? "password" : "text"}
        autoComplete="off"
        value={secrets[key] ?? ""}
        onChange={(e) => setSecrets((s) => ({ ...s, [key]: e.target.value }))}
      />
    </div>
  );

  const filteredNodes = nodes.filter((node) => {
    const needle = nodeQuery.trim().toLowerCase();
    if (!needle) return true;
    return [node.address, node.declared_ip, node.verified_ip, node.origin, node.last_error]
      .filter(Boolean)
      .some((value) => value!.toLowerCase().includes(needle));
  });
  const pagedNodes = filteredNodes.slice((nodePage - 1) * nodePageSize, nodePage * nodePageSize);

  async function updateNode(node: ProxyNode, enabled: boolean) {
    setBusy(true);
    try {
      await api.updateProxyNode(node.pool_id, node.id, { enabled });
      toast.success(enabled ? "代理 IP 已启用" : "代理 IP 已停用");
      await loadDetail(node.pool_id);
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function deleteNode(node: ProxyNode) {
    if (!window.confirm(`确定软删除代理 IP「${node.address}」吗？`)) return;
    setBusy(true);
    try {
      await api.deleteProxyNode(node.pool_id, node.id);
      toast.success("代理 IP 已移入回收状态");
      await loadDetail(node.pool_id);
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function batchNodeAction(actionName: "enable" | "disable") {
    if (!current || selectedNodeIDs.length === 0) return;
    setBusy(true);
    try {
      const result = await api.batchUpdateProxyNodes(current.id, selectedNodeIDs, actionName);
      toast.success(`已更新 ${result.changed} 个节点`);
      setSelectedNodeIDs([]);
      await loadDetail(current.id);
    } catch (e) {
      toast.error((e as Error).message || "批量操作失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="min-w-0 space-y-6 p-4 md:p-6">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="font-semibold text-xl">代理池</h1>
          <p className="mt-1 text-muted-foreground text-sm">
            {pools.length} 个来源 · {globalSettings.enabled ? "新任务默认可使用代理" : "新任务默认关闭代理"}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <div className="flex h-9 items-center gap-2 border px-3">
            <span className="whitespace-nowrap text-muted-foreground text-xs">全局代理</span>
            <Switch
              checked={globalSettings.enabled}
              disabled={!admin}
              aria-label="全局代理开关"
              onCheckedChange={(value) => {
                if (!value && !window.confirm("关闭后只影响新任务，已有运行任务继续使用当前租约。确认关闭吗？")) return;
                void updateGlobalSettings({ enabled: value });
              }}
            />
          </div>
          <select
            className="h-9 max-w-52 rounded-md border bg-background px-3 text-sm"
            value={globalSettings.default_pool_id ?? ""}
            disabled={!admin}
            aria-label="默认代理池"
            onChange={(event) =>
              void updateGlobalSettings({ default_pool_id: event.target.value ? Number(event.target.value) : 0 })
            }
          >
            <option value="">默认代理池：不指定</option>
            {pools
              .filter((pool) => pool.config.enabled)
              .map((pool) => (
                <option key={pool.id} value={pool.id}>
                  {pool.config.name}
                </option>
              ))}
          </select>
          <Button
            variant="outline"
            size="icon"
            aria-label="刷新代理来源"
            title="刷新代理来源"
            disabled={loading || busy}
            onClick={() => {
              void load();
              void loadDiscoverySummary();
              if (selected) void loadDetail(selected);
            }}
          >
            <RefreshCwIcon className="size-4" />
          </Button>
          {admin && (
            <Button onClick={() => edit()}>
              <PlusIcon className="mr-2 size-4" />
              添加来源
            </Button>
          )}
        </div>
      </header>
      {error && (
        <p role="alert" className="text-destructive text-sm">
          {error}
        </p>
      )}
      <Tabs value={mainView} onValueChange={setMainView} className="max-w-full overflow-x-auto">
        <TabsList>
          <TabsTrigger value="overview">总览</TabsTrigger>
          <TabsTrigger value="nodes">代理 IP</TabsTrigger>
          <TabsTrigger value="sources">来源管理</TabsTrigger>
          <TabsTrigger value="discovery">FOFA / Quake 发现</TabsTrigger>
          <TabsTrigger value="events">使用记录</TabsTrigger>
        </TabsList>
      </Tabs>
      {mainView === "discovery" && admin ? (
        <ProxyDiscoveryPanel
          refreshKey={discoveryRefresh}
          onCreatePool={() => edit()}
          onOpenPool={(id) => {
            setSelected(id);
            setMainView("nodes");
            void load();
            void loadDetail(id);
          }}
          onChanged={() => {
            void loadDiscoverySummary();
            void load();
            if (selected) void loadDetail(selected);
          }}
        />
      ) : null}
      {mainView !== "discovery" ? (
        <>
          {mainView === "overview" && admin ? (
            <div className="flex flex-wrap items-center justify-between gap-3 border-b pb-4">
              <div>
                <p className="font-medium">FOFA / Quake 发现候选</p>
                <p className="mt-1 text-muted-foreground text-sm">
                  {discoverySummary
                    ? `${discoverySummary.total} 条发现记录 · ${discoverySummary.unapproved} 条未入池 · ${discoverySummary.approved} 条已入池`
                    : "发现统计暂不可用"}
                </p>
              </div>
              <Button variant="outline" onClick={() => setMainView("discovery")}>
                查看发现候选
              </Button>
            </div>
          ) : null}
          <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-6">
            {(
              [
                ["全部节点", summary.total, ActivityIcon, "text-foreground"],
                ["可用", summary.available, CheckCircle2Icon, "text-emerald-600"],
                ["待验证", summary.pending, SearchIcon, "text-muted-foreground"],
                ["即将过期", summary.expiring, GaugeIcon, "text-amber-600"],
                ["异常", summary.error, WifiOffIcon, "text-destructive"],
                ["已停用", summary.disabled, WifiOffIcon, "text-muted-foreground"],
              ] as [string, number, LucideIcon, string][]
            ).map(([label, value, Icon, color]) => (
              <div key={String(label)} className="border bg-card p-4">
                <div className="flex items-center justify-between gap-3">
                  <span className="text-muted-foreground text-sm">{label}</span>
                  <Icon className={`size-4 ${color}`} />
                </div>
                <p className="mt-2 font-semibold text-2xl tabular-nums">{value}</p>
                <p className="mt-1 text-muted-foreground text-xs">{current?.config.name ?? "请选择代理池"}</p>
              </div>
            ))}
          </section>
          <div
            className={
              mainView === "nodes" || mainView === "overview" || mainView === "sources"
                ? "overflow-x-auto border-y"
                : "hidden"
            }
          >
            <table className="w-full min-w-[760px] text-left text-sm">
              <thead className="text-muted-foreground">
                <tr>
                  <th className="p-3">来源</th>
                  <th className="p-3">类型</th>
                  <th className="p-3">状态</th>
                  <th className="p-3">套餐余量</th>
                  <th className="p-3">更新时间</th>
                  <th className="p-3 text-right">操作</th>
                </tr>
              </thead>
              <tbody>
                {pools.map((p) => (
                  <tr key={p.id} className={`border-t ${selected === p.id ? "bg-muted/60" : ""}`}>
                    <td className="min-w-48 max-w-64 p-3">
                      <button
                        type="button"
                        className="break-all text-left font-medium underline-offset-4 hover:underline"
                        onClick={() => setSelected(p.id)}
                      >
                        {p.config.name}
                      </button>
                    </td>
                    <td className="whitespace-nowrap p-3">{kinds[p.config.kind]}</td>
                    <td className="p-3">{p.config.enabled ? "启用" : "禁用"}</td>
                    <td
                      className={`p-3 ${!p.quota.error && p.quota.remaining !== undefined && p.quota.remaining <= p.config.low_quota ? "text-amber-700 dark:text-amber-400" : ""}`}
                    >
                      {!p.config.quota_supported
                        ? "不支持查询"
                        : p.quota.error
                          ? "未知"
                          : p.quota.unavailable
                            ? "暂无余量数据"
                            : p.quota.remaining === undefined
                              ? "未查询"
                              : `${p.quota.remaining} / ${p.quota.total}`}
                    </td>
                    <td className="whitespace-nowrap p-3 text-xs">{when(p.quota.checked_at)}</td>
                    <td className="p-3">
                      <div className="flex justify-end gap-1">
                        {admin && (
                          <>
                            <Button
                              variant="ghost"
                              size="icon"
                              title="编辑来源"
                              aria-label={`编辑 ${p.config.name}`}
                              disabled={busy}
                              onClick={() => edit(p)}
                            >
                              <PencilIcon className="size-4" />
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon"
                              title="删除来源"
                              aria-label={`删除 ${p.config.name}`}
                              disabled={busy}
                              onClick={() => setDeleting(p)}
                            >
                              <Trash2Icon className="size-4" />
                            </Button>
                          </>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {pools.length === 0 && (
              <p className="p-10 text-center text-muted-foreground text-sm">{loading ? "加载中" : "暂无代理来源"}</p>
            )}
          </div>
          {current && admin && mainView !== "sources" && (
            <section className="space-y-4">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <h2 className="break-all font-medium">{current.config.name}</h2>
                <div className="flex flex-wrap gap-2">
                  {current.config.quota_supported && (
                    <Button
                      variant="outline"
                      disabled={busy || !current.config.enabled}
                      onClick={() =>
                        action(async () => {
                          const q = await api.proxyQuota(current.id);
                          if (q.error) {
                            await load();
                            throw new Error(q.error);
                          }
                        }, "余量已更新")
                      }
                    >
                      <RefreshCwIcon className="mr-2 size-4" />
                      查询余量
                    </Button>
                  )}
                  <Button
                    variant="outline"
                    disabled={busy || !current.config.enabled}
                    onClick={() => action(() => api.fetchProxyNodes(current.id), "节点已更新")}
                  >
                    <DownloadIcon className="mr-2 size-4" />
                    {current.config.kind === "fixed" ? "刷新节点" : "提取节点"}
                  </Button>
                </div>
              </div>
              {current.quota.error && (
                <p role="alert" className="text-destructive text-sm">
                  {current.quota.error}
                </p>
              )}
              {current.quota.unavailable && !current.quota.error && (
                <p role="status" className="text-muted-foreground text-sm">
                  小象暂未返回余量数据。余量查询仅适用于短效包量套餐；其他套餐可关闭余量查询，继续提取节点。
                </p>
              )}
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="relative min-w-64 flex-1 sm:max-w-sm">
                  <SearchIcon className="pointer-events-none absolute top-2.5 left-3 size-4 text-muted-foreground" />
                  <Input
                    value={nodeQuery}
                    onChange={(event) => {
                      setNodeQuery(event.target.value);
                      setNodePage(1);
                    }}
                    placeholder="搜索 IP、协议、出口或错误"
                    className="pl-9"
                  />
                </div>
                <p className="text-muted-foreground text-xs">
                  最后检查：
                  {when(
                    nodes.reduce<string | undefined>((latest, node) => {
                      if (!node.last_checked_at) return latest;
                      return !latest || Date.parse(node.last_checked_at) > Date.parse(latest)
                        ? node.last_checked_at
                        : latest;
                    }, undefined),
                  )}
                </p>
              </div>
              {selectedNodeIDs.length > 0 ? (
                <div className="flex flex-wrap items-center gap-2 border bg-muted/30 px-3 py-2">
                  <span className="text-sm">已选择 {selectedNodeIDs.length} 个节点</span>
                  <Button size="sm" variant="outline" disabled={busy} onClick={() => void batchNodeAction("enable")}>
                    批量启用
                  </Button>
                  <Button size="sm" variant="outline" disabled={busy} onClick={() => void batchNodeAction("disable")}>
                    批量停用
                  </Button>
                  <Button size="sm" variant="ghost" disabled={busy} onClick={() => setSelectedNodeIDs([])}>
                    清除选择
                  </Button>
                </div>
              ) : null}
              <Tabs defaultValue="nodes">
                <TabsList>
                  <TabsTrigger value="nodes">节点</TabsTrigger>
                  <TabsTrigger value="events">出口审计</TabsTrigger>
                </TabsList>
                <TabsContent value="nodes">
                  <div className="overflow-x-auto">
                    <table className="w-full min-w-[780px] text-left text-sm">
                      <thead className="border-b text-muted-foreground">
                        <tr>
                          <th className="p-3">
                            <Checkbox
                              checked={
                                pagedNodes.length > 0 && pagedNodes.every((node) => selectedNodeIDs.includes(node.id))
                              }
                              onCheckedChange={(checked) => {
                                setSelectedNodeIDs((currentIDs) =>
                                  checked
                                    ? Array.from(new Set([...currentIDs, ...pagedNodes.map((node) => node.id)]))
                                    : currentIDs.filter((id) => !pagedNodes.some((node) => node.id === id)),
                                );
                              }}
                              aria-label="选择本页节点"
                            />
                          </th>
                          <th className="p-3">代理节点</th>
                          <th className="p-3">来源</th>
                          <th className="p-3">状态</th>
                          <th className="p-3">到期时间</th>
                          <th className="p-3">验证耗时</th>
                          <th className="p-3">声明出口</th>
                          <th className="p-3">最近实测出口</th>
                          <th className="p-3">成功率</th>
                          <th className="p-3 text-right">操作</th>
                        </tr>
                      </thead>
                      <tbody>
                        {pagedNodes.map((n) => (
                          <tr key={n.id} className="border-b">
                            <td className="p-3">
                              <Checkbox
                                checked={selectedNodeIDs.includes(n.id)}
                                onCheckedChange={(checked) =>
                                  setSelectedNodeIDs((currentIDs) =>
                                    checked ? [...currentIDs, n.id] : currentIDs.filter((id) => id !== n.id),
                                  )
                                }
                                aria-label={`选择节点 ${n.id}`}
                              />
                            </td>
                            <td className="max-w-64 break-all p-3 font-mono text-xs">
                              {n.address}
                              {n.last_error && (
                                <p className="mt-1 break-words font-sans text-destructive">
                                  {proxyVerificationMessage(n.last_error)}
                                </p>
                              )}
                            </td>
                            <td className="p-3 text-xs">
                              {n.origin === "fofa" ? "FOFA" : n.origin === "quake" ? "Quake" : "供应商"}
                            </td>
                            <td
                              className={`whitespace-nowrap p-3 text-xs ${nodeStatus(n, current.config.expiry_warning_seconds || 3600).className}`}
                            >
                              {nodeStatus(n, current.config.expiry_warning_seconds || 3600).label}
                            </td>
                            <td
                              className={`whitespace-nowrap p-3 text-xs ${n.expires_at && Date.parse(n.expires_at) <= Date.now() ? "text-destructive" : ""}`}
                            >
                              {n.expires_at ? when(n.expires_at) : "固定节点"}
                            </td>
                            <td className="whitespace-nowrap p-3 text-xs">
                              {n.latency_ms != null ? `${n.latency_ms} ms` : "-"}
                            </td>
                            <td className="p-3">{n.declared_ip || "未知"}</td>
                            <td className="p-3">{n.verified_ip || "未验证"}</td>
                            <td className="whitespace-nowrap p-3 text-xs">
                              {n.success_count + n.failure_count > 0
                                ? `${Math.round((n.success_count / (n.success_count + n.failure_count)) * 100)}%`
                                : "暂无"}
                            </td>
                            <td className="p-3">
                              <div className="flex items-center justify-end gap-1">
                                <Switch
                                  checked={n.enabled}
                                  disabled={busy || !admin}
                                  aria-label={`${n.enabled ? "停用" : "启用"} ${n.address}`}
                                  onCheckedChange={(value) => void updateNode(n, value)}
                                />
                                <Button
                                  size="icon"
                                  variant="ghost"
                                  title={current.config.verify_url ? "验证出口" : "请先配置验证 URL"}
                                  aria-label={`验证节点 ${n.id}`}
                                  disabled={busy || !current.config.enabled || !current.config.verify_url}
                                  onClick={() => action(() => api.verifyProxyNode(current.id, n.id), "出口已验证")}
                                >
                                  <ShieldCheckIcon className="size-4" />
                                </Button>
                                <Button
                                  size="icon"
                                  variant="ghost"
                                  title="软删除节点"
                                  aria-label={`删除节点 ${n.id}`}
                                  disabled={busy || !admin}
                                  onClick={() => void deleteNode(n)}
                                >
                                  <Trash2Icon className="size-4 text-destructive" />
                                </Button>
                              </div>
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                    {filteredNodes.length === 0 && (
                      <p className="p-8 text-center text-muted-foreground text-sm">
                        {nodes.length === 0 ? "暂无节点" : "没有匹配的节点"}
                      </p>
                    )}
                  </div>
                  <TablePagination
                    page={nodePage}
                    pageSize={nodePageSize}
                    total={filteredNodes.length}
                    onPageChange={setNodePage}
                    onPageSizeChange={(size) => {
                      setNodePageSize(size);
                      setNodePage(1);
                    }}
                  />
                </TabsContent>
                <TabsContent value="events">
                  <ProxyEventsTable events={events} />
                </TabsContent>
              </Tabs>
            </section>
          )}
        </>
      ) : null}
      <Sheet
        open={open}
        onOpenChange={(v) => {
          setOpen(v);
          if (!v) setSecrets({});
        }}
      >
        <SheetContent className="w-full overflow-y-auto sm:max-w-xl">
          <SheetHeader>
            <SheetTitle>{editing ? "编辑代理来源" : "添加代理来源"}</SheetTitle>
            <SheetDescription>来源配置与连接认证</SheetDescription>
          </SheetHeader>
          <form onSubmit={save} className="space-y-5 p-4">
            <div className="space-y-2">
              <Label htmlFor="pool-name">名称</Label>
              <Input
                id="pool-name"
                required
                maxLength={80}
                value={config.name}
                onChange={(e) => field("name", e.target.value)}
              />
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="pool-kind">来源类型</Label>
                <select
                  id="pool-kind"
                  className={selectClass}
                  value={config.kind}
                  onChange={(e) => {
                    field("kind", e.target.value as ProxyPoolConfig["kind"]);
                    if (e.target.value === "xiaoxiang" && config.protocol === "https") field("protocol", "http");
                  }}
                >
                  {Object.entries(kinds).map(([k, label]) => (
                    <option key={k} value={k}>
                      {label}
                    </option>
                  ))}
                </select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="pool-protocol">代理协议</Label>
                <select
                  id="pool-protocol"
                  className={selectClass}
                  value={config.protocol}
                  onChange={(e) => field("protocol", e.target.value as ProxyPoolConfig["protocol"])}
                >
                  <option value="http">HTTP</option>
                  {config.kind !== "xiaoxiang" && <option value="https">HTTPS</option>}
                  <option value="socks5">SOCKS5</option>
                </select>
              </div>
            </div>
            <div className="flex items-center justify-between">
              <Label htmlFor="pool-enabled">启用来源</Label>
              <Switch id="pool-enabled" checked={config.enabled} onCheckedChange={(v) => field("enabled", v)} />
            </div>
            {config.kind === "fixed" && (
              <div className="space-y-2">
                <Label htmlFor="pool-addresses">代理地址（可选，每行一个）</Label>
                <Textarea
                  id="pool-addresses"
                  rows={5}
                  placeholder="留空创建用于接收发现候选的代理池"
                  value={addresses}
                  onChange={(e) => setAddresses(e.target.value)}
                />
              </div>
            )}
            {config.kind === "generic" && (
              <>
                <div className="space-y-2">
                  <Label htmlFor="pool-api">API URL</Label>
                  <Input
                    id="pool-api"
                    type="url"
                    required
                    value={config.api_url}
                    onChange={(e) => field("api_url", e.target.value)}
                  />
                </div>
                <div className="grid grid-cols-2 gap-4">
                  {(
                    [
                      ["items_path", "节点数组路径"],
                      ["host_field", "主机字段"],
                      ["port_field", "端口字段"],
                    ] as const
                  ).map(([k, label]) => (
                    <div key={k} className="space-y-2">
                      <Label htmlFor={k}>{label}</Label>
                      <Input id={k} value={config[k]} onChange={(e) => field(k, e.target.value)} />
                    </div>
                  ))}
                  <div className="space-y-2">
                    <Label htmlFor="pool-ttl">节点有效期（秒）</Label>
                    <Input
                      id="pool-ttl"
                      type="number"
                      min={10}
                      required
                      value={config.ttl_seconds}
                      onChange={(e) => field("ttl_seconds", Number(e.target.value))}
                    />
                  </div>
                </div>
              </>
            )}
            <div className="grid grid-cols-2 gap-4">
              {config.kind === "xiaoxiang" && (
                <div className="space-y-2">
                  <Label htmlFor="pool-count">每次提取数量</Label>
                  <Input
                    id="pool-count"
                    type="number"
                    min={1}
                    max={100}
                    required
                    value={config.fetch_count}
                    onChange={(e) => field("fetch_count", Number(e.target.value))}
                  />
                </div>
              )}
              <div className="space-y-2">
                <Label htmlFor="pool-margin">到期余量（秒）</Label>
                <Input
                  id="pool-margin"
                  type="number"
                  min={0}
                  max={3600}
                  required
                  value={config.safety_seconds}
                  onChange={(e) => field("safety_seconds", Number(e.target.value))}
                />
              </div>
            </div>
            {config.kind === "xiaoxiang" && (
              <>
                <div className="flex items-center gap-2">
                  <Checkbox
                    id="pool-quota"
                    checked={config.quota_supported}
                    onCheckedChange={(v) => field("quota_supported", v === true)}
                  />
                  <Label htmlFor="pool-quota">短效包量套餐余量查询</Label>
                </div>
                {config.quota_supported && (
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <Label htmlFor="pool-quota-keys">余量认证参数</Label>
                      <select
                        id="pool-quota-keys"
                        className={selectClass}
                        value={config.quota_key_style}
                        onChange={(e) => field("quota_key_style", e.target.value as "app" | "user")}
                      >
                        <option value="app">appKey / appSecret</option>
                        <option value="user">userKey / userSecret</option>
                      </select>
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="pool-low">低余量阈值</Label>
                      <Input
                        id="pool-low"
                        type="number"
                        min={0}
                        value={config.low_quota}
                        onChange={(e) => field("low_quota", Number(e.target.value))}
                      />
                    </div>
                  </div>
                )}
              </>
            )}
            <div className="space-y-2">
              <Label htmlFor="pool-verify">出口验证 URL（返回纯文本 IP）</Label>
              <Input
                id="pool-verify"
                type="url"
                value={config.verify_url}
                onChange={(e) => field("verify_url", e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="pool-expiry-warning">即将过期提醒（秒）</Label>
              <Input
                id="pool-expiry-warning"
                type="number"
                min={60}
                max={2592000}
                required
                value={config.expiry_warning_seconds}
                onChange={(e) => field("expiry_warning_seconds", Number(e.target.value))}
              />
              <p className="text-muted-foreground text-xs">节点在此时间内到期时显示为“即将过期”。</p>
            </div>
            <fieldset className="space-y-4 border-t pt-4">
              <legend className="font-medium text-sm">凭据</legend>
              {editing && (
                <div className="flex items-center gap-2">
                  <Checkbox
                    id="replace-secrets"
                    checked={replaceSecrets}
                    onCheckedChange={(v) => setReplaceSecrets(v === true)}
                  />
                  <Label htmlFor="replace-secrets">替换整组凭据（空字段会清除）</Label>
                </div>
              )}
              {replaceSecrets && (
                <>
                  {config.kind === "xiaoxiang" && (
                    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                      {secretField("app_key", "AppKey")}
                      {secretField("app_secret", "AppSecret", true)}
                    </div>
                  )}
                  {config.kind === "generic" && (
                    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                      {secretField("api_header", "API 认证 Header")}
                      {secretField("api_value", "API 认证值", true)}
                    </div>
                  )}
                  <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                    {secretField(
                      "proxy_username",
                      config.kind === "xiaoxiang" ? "代理连接账号（可选）" : "代理连接账号",
                    )}
                    {secretField(
                      "proxy_password",
                      config.kind === "xiaoxiang" ? "代理连接密码（可选）" : "代理连接密码",
                      true,
                    )}
                  </div>
                  {config.kind === "xiaoxiang" && (
                    <p className="text-muted-foreground text-xs">
                      连接账号和密码同时留空时，使用 AppKey / AppSecret。
                    </p>
                  )}
                </>
              )}
            </fieldset>
            <Button className="w-full" type="submit" disabled={busy}>
              {busy ? "保存中" : "保存来源"}
            </Button>
          </form>
        </SheetContent>
      </Sheet>
      <AlertDialog
        open={deleting !== null}
        onOpenChange={(v) => {
          if (!v) setDeleting(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除代理来源？</AlertDialogTitle>
            <AlertDialogDescription>
              {deleting?.config.name} 的节点将一并移除。已有任务绑定的来源只能禁用。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={busy}>取消</AlertDialogCancel>
            <AlertDialogAction
              disabled={busy}
              onClick={() => {
                const p = deleting;
                if (p) void action(() => api.deleteProxyPool(p.id), "来源已删除");
                setDeleting(null);
              }}
            >
              删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </main>
  );
}
