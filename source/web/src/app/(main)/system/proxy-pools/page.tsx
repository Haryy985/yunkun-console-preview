"use client";

import * as React from "react";
import {
  ActivityIcon,
  CheckCircle2Icon,
  DownloadIcon,
  FileUpIcon,
  FileDownIcon,
  GaugeIcon,
  LoaderCircleIcon,
  PencilIcon,
  PlusIcon,
  RefreshCwIcon,
  SearchIcon,
  ShieldCheckIcon,
  SquareIcon,
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
  ProxyVerificationMode,
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
const compactSelectClass = "h-9 max-w-full rounded-md border bg-background px-3 text-sm";
const selectClass = `${compactSelectClass} w-full`;
function when(value?: string) {
  return value ? new Date(value).toLocaleString() : "-";
}

function nodeEndpoint(address: string) {
  try {
    const url = new URL(address);
    return { host: url.hostname, port: url.port, protocol: url.protocol.replace(":", "").toUpperCase() };
  } catch {
    return { host: address, port: "-", protocol: "-" };
  }
}

function nodeOrigin(node: ProxyNode, pool: ProxyPool) {
  if (node.origin === "fofa") return "FOFA";
  if (node.origin === "quake") return "Quake";
  return node.origin === "manual" || pool.config.kind === "fixed" ? "手动" : "供应商";
}

function verificationLabel(node: ProxyNode) {
  if (node.last_verify_mode === "connectivity") return "百度连通性";
  return node.last_verify_mode === "exit_ip" || node.verified_at ? "出口 IP" : "-";
}

function nodeStatus(node: ProxyNode, warningSeconds: number) {
  if (node.enabled && node.runtime_cooldown_until && new Date(node.runtime_cooldown_until).getTime() > Date.now())
    return { label: "故障冷却", className: "text-destructive" };
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
  const [recoverNoAuth, setRecoverNoAuth] = React.useState(false);
  const [deleting, setDeleting] = React.useState<ProxyPool | null>(null);
  const [nodeQuery, setNodeQuery] = React.useState("");
  const [selectedNodeIDs, setSelectedNodeIDs] = React.useState<number[]>([]);
  const [mainView, setMainView] = React.useState("nodes");
  const [discoverySummary, setDiscoverySummary] = React.useState<{
    total: number;
    unapproved: number;
    approved: number;
  } | null>(null);
  const [discoveryRefresh, setDiscoveryRefresh] = React.useState(0);
  const [nodePage, setNodePage] = React.useState(1);
  const [nodePageSize, setNodePageSize] = React.useState(20);
  const [nodeTotal, setNodeTotal] = React.useState(0);
  const [nodeStatusFilter, setNodeStatusFilter] = React.useState("");
  const [nodeProtocol, setNodeProtocol] = React.useState("");
  const [nodeSort, setNodeSort] = React.useState("newest");
  const [search, setSearch] = React.useState("");
  const [detailLoading, setDetailLoading] = React.useState(false);
  const [nodesLoaded, setNodesLoaded] = React.useState(false);
  const [detailError, setDetailError] = React.useState("");
  const [eventsError, setEventsError] = React.useState("");
  const [importError, setImportError] = React.useState("");
  const [verifyMode, setVerifyMode] = React.useState<ProxyVerificationMode>("connectivity");
  const [verifyingIDs, setVerifyingIDs] = React.useState<number[]>([]);
  const [verification, setVerification] = React.useState<{
    total: number;
    done: number;
    passed: number;
    failed: number;
    running: boolean;
    canceled: boolean;
  } | null>(null);
  const verificationController = React.useRef<AbortController | null>(null);
  const importInput = React.useRef<HTMLInputElement>(null);
  const [importPending, setImportPending] = React.useState(0);
  const [globalSettings, setGlobalSettings] = React.useState<ProxyGlobalSettings>({ enabled: true });
  const detailRequest = React.useRef(0);
  const previousPool = React.useRef(selected);
  const current = pools.find((p) => p.id === selected);
  const credentialsUnavailable = current?.credentials_available === false;
  const editingCredentialsUnavailable = pools.find((p) => p.id === editing)?.credentials_available === false;

  React.useEffect(() => {
    const timer = window.setTimeout(() => {
      setSearch(nodeQuery.trim());
      setNodePage(1);
    }, 250);
    return () => window.clearTimeout(timer);
  }, [nodeQuery]);
  React.useEffect(() => () => verificationController.current?.abort(), []);

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
  const loadDetail = React.useCallback(
    async (id: number) => {
      const request = ++detailRequest.current;
      setDetailLoading(true);
      const results = await Promise.allSettled([
        api.proxyNodes(id, {
          page: nodePage,
          page_size: nodePageSize,
          q: search,
          status: nodeStatusFilter,
          protocol: nodeProtocol,
          sort: nodeSort,
        }),
        api.proxyEvents(id),
      ]);
      if (request !== detailRequest.current) return;
      setDetailLoading(false);
      const [n, e] = results;
      if (n.status === "fulfilled") {
        setNodes(n.value.nodes);
        setSummary(n.value.summary);
        setNodeTotal(n.value.total);
        setNodesLoaded(true);
        setDetailError("");
      } else {
        setNodes([]);
        setNodeTotal(0);
        setNodesLoaded(false);
        setDetailError(n.reason?.message || "节点读取失败");
      }
      if (e.status === "fulfilled") {
        setEvents(e.value.events);
        setEventsError("");
      } else {
        setEvents([]);
        setEventsError(e.reason?.message || "使用记录读取失败");
      }
    },
    [nodePage, nodePageSize, search, nodeStatusFilter, nodeProtocol, nodeSort],
  );

  React.useEffect(() => {
    if (!selected || !admin) return;
    let canceled = false;
    let previous = 0;
    let failures = 0;
    let timer: ReturnType<typeof setTimeout>;
    setImportPending(0);
    setImportError("");
    const poll = async () => {
      try {
        const status = await api.proxyImportStatus(selected);
        if (canceled) return;
        failures = 0;
        setImportError("");
        setImportPending(status.pending);
        if (status.pending > 0 || previous > 0) await loadDetail(selected);
        previous = status.pending;
      } catch (e) {
        if (canceled) return;
        failures++;
        setImportError((e as Error).message || "导入验证状态读取失败");
      } finally {
        if (!canceled) timer = setTimeout(poll, Math.min(30000, 2000 * 2 ** Math.min(failures, 4)));
      }
    };
    void poll();
    return () => {
      canceled = true;
      clearTimeout(timer);
    };
  }, [selected, admin, loadDetail]);
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
    if (previousPool.current === selected) return;
    previousPool.current = selected;
    setNodes([]);
    setNodesLoaded(false);
    setDetailError("");
    setEventsError("");
    setSelectedNodeIDs([]);
    setNodePage(1);
    setNodeTotal(0);
    setVerification(null);
    setSummary({ total: 0, available: 0, pending: 0, expiring: 0, error: 0, disabled: 0 });
    setEvents([]);
  }, [selected]);
  React.useEffect(() => {
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
    setRecoverNoAuth(false);
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
    if (editingCredentialsUnavailable && !replaceSecrets) {
      toast.error("原代理池凭据不可读，请先恢复原密钥，或勾选替换整组凭据");
      return;
    }
    setBusy(true);
    try {
      if (editingCredentialsUnavailable && editing) {
        await api.recoverProxyCredentials(editing, recoverNoAuth ? {} : secrets, recoverNoAuth);
        setOpen(false);
        setSecrets({});
        await load();
        await loadDetail(editing);
        setDiscoveryRefresh((value) => value + 1);
        toast.success("凭据已恢复，节点已保留；请重新验证并启用");
        return;
      }
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
      await loadDetail(result.id);
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

  const pagedNodes = nodes;

  async function verifyNodes(ids: number[]) {
    if (!current || ids.length === 0 || verificationController.current) return;
    const poolID = current.id;
    const queue = [...new Set(ids)];
    const controller = new AbortController();
    verificationController.current = controller;
    setBusy(true);
    setVerification({ total: queue.length, done: 0, passed: 0, failed: 0, running: true, canceled: false });
    let index = 0;
    try {
      await Promise.all(
        Array.from({ length: Math.min(4, queue.length) }, async () => {
          while (!controller.signal.aborted && index < queue.length) {
            const id = queue[index++];
            setVerifyingIDs((previous) => [...previous, id]);
            let passed = false;
            try {
              await api.verifyProxyNode(poolID, id, verifyMode, controller.signal);
              passed = true;
            } catch (error) {
              if (!controller.signal.aborted && queue.length === 1) {
                toast.error(proxyVerificationMessage((error as Error).message));
              }
            } finally {
              setVerifyingIDs((previous) => previous.filter((value) => value !== id));
              if (!controller.signal.aborted)
                setVerification(
                  (previous) =>
                    previous && {
                      ...previous,
                      done: previous.done + 1,
                      passed: previous.passed + (passed ? 1 : 0),
                      failed: previous.failed + (passed ? 0 : 1),
                    },
                );
            }
          }
        }),
      );
    } finally {
      setVerification((previous) => previous && { ...previous, running: false, canceled: controller.signal.aborted });
      verificationController.current = null;
      await loadDetail(poolID);
      setBusy(false);
    }
  }

  async function importProxyFile(file: File) {
    if (!current || busy) return;
    setBusy(true);
    try {
      const result = await api.importProxyNodes(current.id, file);
      toast.success(`新增 ${result.imported} 个节点，跳过 ${result.skipped} 个重复节点`);
      setImportPending((previous) => previous + result.imported);
      await loadDetail(current.id);
    } catch (error) {
      toast.error((error as Error).message || "代理节点导入失败");
      await loadDetail(current.id);
    } finally {
      setBusy(false);
    }
  }

  async function exportProxyFile() {
    if (!current || busy) return;
    setBusy(true);
    try {
      const blob = await api.exportProxyNodes(current.id);
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = `proxy-pool-${current.id}.csv`;
      link.click();
      URL.revokeObjectURL(url);
      toast.success("代理节点已导出");
    } catch (error) {
      toast.error((error as Error).message || "代理节点导出失败");
    } finally {
      setBusy(false);
    }
  }

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
    if (selectedNodeIDs.length > 500) {
      toast.error("每次最多操作 500 个节点");
      return;
    }
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
    <main data-content-padding="false" className="min-w-0 space-y-4 p-4 md:p-6">
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
              disabled={!admin || busy}
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
            disabled={!admin || busy}
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
            <Button disabled={busy} onClick={() => edit()}>
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
      {credentialsUnavailable && (
        <div role="alert" className="flex flex-wrap items-center gap-3 border-l-2 border-destructive px-3 py-2 text-sm">
          <p className="min-w-0 flex-1 text-destructive">
            当前代理池凭据不可解密。节点数据仍保留，但验证和出网已阻止；这不是 FOFA / Quake 发现来源的 API Key。
          </p>
          <Button variant="outline" disabled={busy} onClick={() => edit(current)}>
            编辑代理池凭据
          </Button>
        </div>
      )}
      <Tabs
        value={mainView}
        onValueChange={(view) => {
          if (!busy) setMainView(view);
        }}
        className="max-w-full overflow-x-auto"
      >
        <TabsList>
          <TabsTrigger value="nodes" disabled={busy}>
            代理 IP
          </TabsTrigger>
          <TabsTrigger value="overview" disabled={busy}>
            总览
          </TabsTrigger>
          <TabsTrigger value="sources" disabled={busy}>
            来源管理
          </TabsTrigger>
          <TabsTrigger value="discovery" disabled={busy}>
            FOFA / Quake 发现
          </TabsTrigger>
          <TabsTrigger value="events" disabled={busy}>
            使用记录
          </TabsTrigger>
        </TabsList>
      </Tabs>
      {mainView === "discovery" && admin ? (
        <ProxyDiscoveryPanel
          refreshKey={discoveryRefresh}
          preferredPoolID={selected}
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
          <section className="grid grid-cols-2 border-y sm:grid-cols-3 xl:grid-cols-6">
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
              <div key={String(label)} className="flex items-center justify-between gap-3 px-3 py-2.5">
                <div className="flex items-center justify-between gap-3">
                  <Icon className={`size-4 ${color}`} />
                  <span className="whitespace-nowrap text-muted-foreground text-xs">{label}</span>
                </div>
                <p className="font-semibold text-lg tabular-nums">{nodesLoaded ? value : "-"}</p>
              </div>
            ))}
          </section>
          <div className={mainView === "overview" || mainView === "sources" ? "overflow-x-auto border-y" : "hidden"}>
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
                        disabled={busy}
                        onClick={() => setSelected(p.id)}
                      >
                        {p.config.name}
                      </button>
                    </td>
                    <td className="whitespace-nowrap p-3">{kinds[p.config.kind]}</td>
                    <td className="p-3">
                      {p.config.enabled ? "启用" : "禁用"}
                      {p.credentials_available === false && <p className="text-destructive text-xs">凭据不可解密</p>}
                    </td>
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
          {pools.length === 0 && mainView === "nodes" && (
            <p className="py-12 text-center text-muted-foreground text-sm">{loading ? "加载中" : "暂无代理来源"}</p>
          )}
          {current && admin && mainView !== "sources" && (
            <section className="space-y-4">
              <div className="flex flex-wrap items-center justify-between gap-3 border-b bg-muted/10 pb-3">
                <div className="flex min-w-0 flex-wrap items-center gap-2">
                  <Label htmlFor="active-pool">代理池</Label>
                  <select
                    id="active-pool"
                    className={`${compactSelectClass} max-w-56`}
                    value={selected ?? ""}
                    disabled={busy}
                    onChange={(event) => setSelected(Number(event.target.value))}
                  >
                    {pools.map((pool) => (
                      <option key={pool.id} value={pool.id}>
                        {pool.config.name}
                      </option>
                    ))}
                  </select>
                  <Button
                    variant="ghost"
                    size="icon"
                    title="编辑当前来源"
                    aria-label="编辑当前来源"
                    disabled={busy}
                    onClick={() => edit(current)}
                  >
                    <PencilIcon className="size-4" />
                  </Button>
                  <span
                    className={`text-xs ${current.config.enabled ? "text-emerald-700 dark:text-emerald-400" : "text-muted-foreground"}`}
                  >
                    {current.config.enabled ? "来源已启用" : "来源已停用"}
                  </span>
                </div>
                {mainView !== "events" && (
                  <div className="flex flex-wrap items-center gap-2">
                    <Label htmlFor="node-verification-mode" className="text-xs">
                      验证目标
                    </Label>
                    <select
                      id="node-verification-mode"
                      className={compactSelectClass}
                      value={verifyMode}
                      disabled={busy}
                      onChange={(event) => setVerifyMode(event.target.value as ProxyVerificationMode)}
                    >
                      <option value="connectivity">百度 · www.baidu.com</option>
                      <option value="exit_ip">出口 IP · IP 回显接口</option>
                    </select>
                    <Button
                      disabled={
                        busy ||
                        detailLoading ||
                        credentialsUnavailable ||
                        !current.config.enabled ||
                        pagedNodes.length === 0
                      }
                      onClick={() => void verifyNodes(pagedNodes.map((node) => node.id))}
                    >
                      <ShieldCheckIcon className="mr-2 size-4" />
                      验证本页
                    </Button>
                    {selectedNodeIDs.length > 0 && (
                      <Button
                        variant="outline"
                        disabled={busy || credentialsUnavailable || !current.config.enabled}
                        onClick={() => void verifyNodes(selectedNodeIDs)}
                      >
                        <ShieldCheckIcon className="mr-2 size-4" />
                        验证所选（{selectedNodeIDs.length}）
                      </Button>
                    )}
                  </div>
                )}
                <div className="flex flex-wrap gap-2">
                  <input
                    ref={importInput}
                    type="file"
                    accept=".txt,.csv,text/plain,text/csv"
                    className="hidden"
                    onChange={(event) => {
                      const file = event.target.files?.[0];
                      event.target.value = "";
                      if (file) void importProxyFile(file);
                    }}
                  />
                  <Button
                    variant="outline"
                    disabled={
                      busy || credentialsUnavailable || !current.config.enabled || current.config.kind !== "fixed"
                    }
                    onClick={() => importInput.current?.click()}
                  >
                    <FileUpIcon className="mr-2 size-4" />
                    导入并验证
                  </Button>
                  <Button variant="outline" disabled={busy} onClick={() => void exportProxyFile()}>
                    <FileDownIcon className="mr-2 size-4" />
                    导出节点
                  </Button>
                  <Button variant="outline" disabled={busy} onClick={() => setMainView("discovery")}>
                    <SearchIcon className="mr-2 size-4" />
                    自动发现
                  </Button>
                  {current.config.quota_supported && (
                    <Button
                      variant="outline"
                      disabled={busy || credentialsUnavailable || !current.config.enabled}
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
                    disabled={busy || credentialsUnavailable || !current.config.enabled}
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
              {detailError && (
                <p role="alert" className="text-sm text-destructive">
                  节点读取失败：{detailError}。无法确定节点数量，请重试。
                </p>
              )}
              {eventsError && mainView === "events" && (
                <p role="alert" className="text-sm text-destructive">
                  使用记录读取失败：{eventsError}
                </p>
              )}
              {importError && (
                <p role="alert" className="text-sm text-destructive">
                  导入验证状态不可用：{importError}
                </p>
              )}
              {!importError && importPending > 0 && (
                <p className="text-sm text-muted-foreground">导入验证中：剩余 {importPending} 个</p>
              )}
              {verification && (
                <div
                  role="status"
                  aria-live="polite"
                  className="flex flex-wrap items-center gap-3 border-primary border-l-2 bg-muted/30 px-3 py-2 text-sm"
                >
                  {verification.running && <LoaderCircleIcon className="size-4 animate-spin" />}
                  <span>
                    {verification.running && "验证中"}
                    {!verification.running && (verification.canceled ? "已停止" : "验证完成")} {verification.done} /{" "}
                    {verification.total}
                  </span>
                  <span className="text-emerald-700 dark:text-emerald-400">通过 {verification.passed}</span>
                  <span className="text-destructive">失败 {verification.failed}</span>
                  {verification.canceled && <span>未完成 {verification.total - verification.done}</span>}
                  {verification.running && (
                    <Button size="sm" variant="outline" onClick={() => verificationController.current?.abort()}>
                      <SquareIcon className="mr-2 size-3" />
                      停止验证
                    </Button>
                  )}
                </div>
              )}
              {mainView !== "events" && (
                <div className="flex flex-wrap items-center gap-2">
                  <div className="relative min-w-0 basis-full sm:max-w-sm sm:flex-1 sm:basis-auto">
                    <SearchIcon className="pointer-events-none absolute top-2.5 left-3 size-4 text-muted-foreground" />
                    <Input
                      value={nodeQuery}
                      onChange={(event) => {
                        setNodeQuery(event.target.value);
                      }}
                      aria-label="搜索代理节点"
                      maxLength={200}
                      disabled={busy}
                      placeholder="搜索 IP、协议、出口或错误"
                      className="pl-9"
                    />
                  </div>
                  <select
                    aria-label="筛选协议"
                    className={compactSelectClass}
                    value={nodeProtocol}
                    disabled={busy}
                    onChange={(event) => {
                      setNodeProtocol(event.target.value);
                      setNodePage(1);
                    }}
                  >
                    <option value="">全部协议</option>
                    <option value="http">HTTP</option>
                    <option value="https">HTTPS</option>
                    <option value="socks5">SOCKS5</option>
                  </select>
                  <select
                    aria-label="筛选节点状态"
                    className={compactSelectClass}
                    value={nodeStatusFilter}
                    disabled={busy}
                    onChange={(event) => {
                      setNodeStatusFilter(event.target.value);
                      setNodePage(1);
                    }}
                  >
                    <option value="">全部状态</option>
                    <option value="available">可用</option>
                    <option value="pending">待验证</option>
                    <option value="expiring">即将过期</option>
                    <option value="error">异常</option>
                    <option value="disabled">已停用</option>
                  </select>
                  <select
                    aria-label="节点排序"
                    className={compactSelectClass}
                    value={nodeSort}
                    disabled={busy}
                    onChange={(event) => {
                      setNodeSort(event.target.value);
                      setNodePage(1);
                    }}
                  >
                    <option value="newest">最新加入</option>
                    <option value="latency">验证耗时升序</option>
                  </select>
                  <span className="text-muted-foreground text-xs">
                    {nodesLoaded ? `共 ${nodeTotal} 条` : "数量未知"}
                  </span>
                </div>
              )}
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
              <Tabs value={mainView === "events" ? "events" : "nodes"}>
                <TabsContent value="nodes">
                  <div className="overflow-x-auto">
                    <table
                      aria-label="代理节点列表"
                      className="w-full min-w-[1000px] text-left text-sm"
                      aria-busy={detailLoading}
                    >
                      <thead className="whitespace-nowrap border-y bg-muted/40 text-muted-foreground">
                        <tr>
                          <th className="p-3">
                            <Checkbox
                              disabled={busy || detailLoading}
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
                          <th className="p-3">Host</th>
                          <th className="p-3">Port</th>
                          <th className="p-3">类型</th>
                          <th className="p-3">来源</th>
                          <th className="p-3">状态</th>
                          <th className="p-3">验证耗时</th>
                          <th className="p-3">最近验证</th>
                          <th className="p-3">出口 IP</th>
                          <th className="sticky right-0 bg-background p-3 text-right">操作</th>
                        </tr>
                      </thead>
                      <tbody>
                        {pagedNodes.map((n) => (
                          <tr key={n.id} className="border-b even:bg-muted/20 hover:bg-muted/40">
                            <td className="p-3">
                              <Checkbox
                                disabled={busy || detailLoading}
                                checked={selectedNodeIDs.includes(n.id)}
                                onCheckedChange={(checked) =>
                                  setSelectedNodeIDs((currentIDs) =>
                                    checked ? [...currentIDs, n.id] : currentIDs.filter((id) => id !== n.id),
                                  )
                                }
                                aria-label={`选择节点 ${n.id}`}
                              />
                            </td>
                            <td className="max-w-52 break-all p-3 font-mono text-xs">
                              {nodeEndpoint(n.address).host}
                              {(n.last_error || n.runtime_error) && (
                                <p
                                  className="mt-1 line-clamp-2 max-w-44 break-words font-sans text-destructive"
                                  title={proxyVerificationMessage(n.last_error || n.runtime_error || "")}
                                >
                                  {proxyVerificationMessage(n.last_error || n.runtime_error || "")}
                                </p>
                              )}
                            </td>
                            <td className="p-3 font-mono text-xs">{nodeEndpoint(n.address).port}</td>
                            <td className="p-3 text-xs">{nodeEndpoint(n.address).protocol}</td>
                            <td className="p-3 text-xs">{nodeOrigin(n, current)}</td>
                            <td
                              className={`whitespace-nowrap p-3 text-xs ${nodeStatus(n, current.config.expiry_warning_seconds || 3600).className}`}
                            >
                              {verifyingIDs.includes(n.id)
                                ? "验证中"
                                : nodeStatus(n, current.config.expiry_warning_seconds || 3600).label}
                              {n.expires_at && (
                                <div className="mt-1 text-muted-foreground" title={`到期时间：${when(n.expires_at)}`}>
                                  {new Date(n.expires_at).toLocaleDateString()}
                                </div>
                              )}
                            </td>
                            <td className="whitespace-nowrap p-3 text-xs">
                              {n.latency_ms != null ? `${n.latency_ms} ms` : "-"}
                            </td>
                            <td
                              className="whitespace-nowrap p-3 text-xs"
                              title={`验证通过 ${n.success_count} 次，失败 ${n.failure_count} 次`}
                            >
                              <div>
                                {verificationLabel(n)}
                                {n.last_http_status ? ` · HTTP ${n.last_http_status}` : ""}
                              </div>
                              <div className="mt-1 text-muted-foreground" title={n.last_verify_target}>
                                {when(n.last_checked_at)}
                              </div>
                            </td>
                            <td
                              className="whitespace-nowrap p-3 font-mono text-xs"
                              title={n.verified_at ? `出口 IP 验证于 ${when(n.verified_at)}` : undefined}
                            >
                              {n.verified_ip || (n.last_verify_mode === "connectivity" ? "未检测出口 IP" : "-")}
                            </td>
                            <td className="sticky right-0 bg-background p-3">
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
                                  title={verifyMode === "connectivity" ? "验证百度连通性" : "验证出口 IP"}
                                  aria-label={`验证节点 ${n.id}`}
                                  disabled={busy || detailLoading || credentialsUnavailable || !current.config.enabled}
                                  onClick={() => void verifyNodes([n.id])}
                                >
                                  {verifyingIDs.includes(n.id) ? (
                                    <LoaderCircleIcon className="size-4 animate-spin" />
                                  ) : (
                                    <ShieldCheckIcon className="size-4" />
                                  )}
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
                    {pagedNodes.length === 0 && (
                      <p className="p-8 text-center text-muted-foreground text-sm">
                        {detailLoading ? "加载中" : detailError ? "节点读取失败，数量未知" : "没有匹配的节点"}
                      </p>
                    )}
                  </div>
                  {nodesLoaded && (
                    <fieldset disabled={busy} className="min-w-0">
                      <TablePagination
                        page={nodePage}
                        pageSize={nodePageSize}
                        total={nodeTotal}
                        onPageChange={(page) => {
                          if (!busy) setNodePage(page);
                        }}
                        pageSizeOptions={[10, 20, 50, 100]}
                        onPageSizeChange={(size) => {
                          if (busy) return;
                          setNodePageSize(size);
                          setNodePage(1);
                        }}
                      />
                    </fieldset>
                  )}
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
            {editingCredentialsUnavailable && (
              <p role="alert" className="text-sm text-destructive">
                原代理池凭据不可读。恢复前请暂停此池关联的全部任务。本次仅替换凭据，保留节点并停用旧验证；其他配置不变。
                仅替换凭据会保留节点，但停用节点并清除旧验证结果；有任务绑定时禁止替换。
              </p>
            )}
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
              <Label htmlFor="pool-verify">出口 IP 验证 URL（可选，纯文本 IP 或 JSON）</Label>
              <Input
                id="pool-verify"
                type="url"
                placeholder="https://api.ipify.org?format=json"
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
              {editing && replaceSecrets && (
                <p className="text-sm text-muted-foreground">
                  仅在确认代理不需要认证时留空。仅替换凭据会停用已有节点，需要重新验证后启用。
                </p>
              )}
              {editingCredentialsUnavailable && replaceSecrets && config.kind === "fixed" && (
                <div className="flex items-center gap-2">
                  <Checkbox id="recover-no-auth" checked={recoverNoAuth}
                    onCheckedChange={(value) => setRecoverNoAuth(value === true)} />
                  <Label htmlFor="recover-no-auth">确认此池代理不需要账号密码</Label>
                </div>
              )}
              {replaceSecrets && !recoverNoAuth && (
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
                    <p className="text-muted-foreground text-xs">连接账号和密码同时留空时，使用 AppKey / AppSecret。</p>
                  )}
                </>
              )}
            </fieldset>
            <Button className="w-full" type="submit" disabled={busy}>
              {busy ? "保存中" : editingCredentialsUnavailable ? "确认恢复凭据" : "保存来源"}
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
