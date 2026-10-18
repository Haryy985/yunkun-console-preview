"use client";

import * as React from "react";

import { ChevronLeftIcon, ChevronRightIcon, PlusIcon, RefreshCwIcon } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Sheet, SheetContent, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import { api } from "@/lib/api";
import type { TaskProfileEntry, TaskProfileInput, TaskProfileKind, TaskScopeRow } from "@/lib/types";

const KINDS: Array<{ value: TaskProfileKind; label: string }> = [
  { value: "host", label: "主机" },
  { value: "application", label: "应用" },
  { value: "route_family", label: "路由族" },
  { value: "identity", label: "身份类型" },
  { value: "tenant", label: "租户边界" },
  { value: "technology", label: "技术" },
  { value: "data_class", label: "数据类别" },
  { value: "trust_boundary", label: "信任边界" },
  { value: "exclusion", label: "排除项" },
  { value: "open_question", label: "待确认问题" },
];

function scopeLabel(scope: TaskScopeRow): string {
  const value = [scope.company_name, scope.domain, scope.net, scope.value, scope.company_id, scope.id].find(
    (item) => item !== undefined && item !== "",
  );
  return `${scope.kind}: ${value}`;
}

export function TargetProfileTab({ taskId }: { taskId: string }) {
  const [entries, setEntries] = React.useState<TaskProfileEntry[]>([]);
  const [scopes, setScopes] = React.useState<TaskScopeRow[]>([]);
  const [admin, setAdmin] = React.useState(false);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState("");
  const [before, setBefore] = React.useState(0);
  const [history, setHistory] = React.useState<number[]>([]);
  const [open, setOpen] = React.useState(false);
  const [saving, setSaving] = React.useState(false);
  const [scopeId, setScopeId] = React.useState("");
  const [kind, setKind] = React.useState<TaskProfileKind>("host");
  const [evidenceKind, setEvidenceKind] = React.useState<"observed" | "inferred">("observed");
  const [label, setLabel] = React.useState("");
  const [summary, setSummary] = React.useState("");
  const [query, setQuery] = React.useState("");
  const [confidence, setConfidence] = React.useState(70);
  const [nodeId, setNodeId] = React.useState("");
  const scopeNames = new Map(scopes.map((scope) => [scope.id, scopeLabel(scope)]));

  const load = React.useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const me = await api.authMe();
      if (me.role !== "admin") {
        setAdmin(false);
        setEntries([]);
        return;
      }
      setAdmin(true);
      const [profile, scope] = await Promise.all([api.taskProfile(taskId, before), api.taskScope(taskId)]);
      setEntries(profile.entries);
      setScopes(scope.scope.filter((item) => String(item.task_id) === taskId));
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }, [taskId, before]);

  React.useEffect(() => {
    void load();
  }, [load]);

  async function save(event: React.FormEvent) {
    event.preventDefault();
    const input: TaskProfileInput = {
      scope_id: Number(scopeId),
      kind,
      label: label.trim(),
      evidence_kind: evidenceKind,
      source_summary: summary.trim(),
      source_query: query.trim(),
      confidence,
      ...(nodeId.trim() ? { source_node_id: Number(nodeId) } : {}),
    };
    setSaving(true);
    try {
      await api.addTaskProfile(taskId, input);
      toast.success("观察记录已保存");
      setOpen(false);
      setLabel("");
      setSummary("");
      setQuery("");
      setNodeId("");
      setBefore(0);
      setHistory([]);
      if (before === 0) await load();
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setSaving(false);
    }
  }

  if (!loading && !admin && !error) {
    return <p className="py-8 text-muted-foreground text-sm">目标画像仅管理员可查看。</p>;
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="font-semibold text-sm">目标画像</h2>
          <p className="text-muted-foreground text-xs">观察与推断单独留存，不会自动扩大任务授权范围。</p>
        </div>
        <div className="flex items-center gap-2">
          <Button size="sm" variant="outline" onClick={() => void load()} disabled={loading} title="刷新画像">
            <RefreshCwIcon />
          </Button>
          <Button size="sm" onClick={() => setOpen(true)} disabled={!admin || scopes.length === 0}>
            <PlusIcon /> 登记观察
          </Button>
        </div>
      </div>
      {error && (
        <p role="alert" className="text-destructive text-sm">
          {error}
        </p>
      )}
      <div className="overflow-x-auto border-y">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>类别 / 目标</TableHead>
              <TableHead>范围</TableHead>
              <TableHead>依据</TableHead>
              <TableHead>置信度</TableHead>
              <TableHead>观察时间</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {entries.map((entry) => (
              <TableRow key={entry.id}>
                <TableCell className="min-w-48 max-w-80">
                  <div className="text-muted-foreground text-xs">
                    {KINDS.find((item) => item.value === entry.kind)?.label}
                  </div>
                  <div className="break-words font-medium">{entry.label}</div>
                  {entry.same_key_count > 1 && (
                    <span className="text-muted-foreground text-xs">同目标 {entry.same_key_count} 条来源</span>
                  )}
                </TableCell>
                <TableCell className="max-w-44 break-words text-xs">
                  {scopeNames.get(entry.scope_id ?? -1) ?? "原范围已移除"}
                </TableCell>
                <TableCell className="min-w-48 max-w-96 text-xs">
                  <div>
                    {entry.evidence_kind === "observed" ? "直接观察" : "推断"} · {entry.source_summary}
                  </div>
                  {entry.source_node_id && (
                    <div className="text-muted-foreground">探索节点 #{entry.source_node_id}</div>
                  )}
                  {entry.source_query && (
                    <div className="break-all text-muted-foreground">查询：{entry.source_query}</div>
                  )}
                </TableCell>
                <TableCell className="tabular-nums">{entry.confidence}%</TableCell>
                <TableCell className="whitespace-nowrap text-xs">
                  {new Date(entry.observed_at).toLocaleString()}
                </TableCell>
              </TableRow>
            ))}
            {!loading && entries.length === 0 && (
              <TableRow>
                <TableCell colSpan={5} className="py-12 text-center text-muted-foreground">
                  暂无观察记录
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
      <div className="flex justify-end gap-2">
        <Button
          size="sm"
          variant="outline"
          disabled={history.length === 0 || loading}
          onClick={() => {
            setBefore(history[history.length - 1]);
            setHistory((old) => old.slice(0, -1));
          }}
          title="上一页"
        >
          <ChevronLeftIcon />
        </Button>
        <Button
          size="sm"
          variant="outline"
          disabled={entries.length < 25 || loading}
          onClick={() => {
            setHistory((old) => [...old, before]);
            setBefore(entries[entries.length - 1].id);
          }}
          title="下一页"
        >
          <ChevronRightIcon />
        </Button>
      </div>
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent className="overflow-y-auto sm:max-w-lg">
          <SheetHeader>
            <SheetTitle>登记目标观察</SheetTitle>
          </SheetHeader>
          <form
            id="target-profile-form"
            onSubmit={(event) => void save(event)}
            className="flex flex-1 flex-col gap-4 overflow-y-auto px-4"
          >
            <div className="space-y-1 text-xs">
              <label htmlFor="profile-scope">关联授权范围</label>
              <Select value={scopeId} onValueChange={setScopeId}>
                <SelectTrigger id="profile-scope" className="w-full">
                  <SelectValue placeholder="选择范围" />
                </SelectTrigger>
                <SelectContent>
                  {scopes.map((scope) => (
                    <SelectItem key={scope.id} value={String(scope.id)}>
                      {scopeLabel(scope)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1 text-xs">
              <label htmlFor="profile-kind">类别</label>
              <Select value={kind} onValueChange={(value) => setKind(value as TaskProfileKind)}>
                <SelectTrigger id="profile-kind" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {KINDS.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1 text-xs">
              <label htmlFor="profile-label">目标或边界</label>
              <Input
                id="profile-label"
                required
                maxLength={240}
                value={label}
                onChange={(event) => setLabel(event.target.value)}
              />
            </div>
            <div className="space-y-1 text-xs">
              <label htmlFor="profile-evidence">依据类型</label>
              <Select value={evidenceKind} onValueChange={(value) => setEvidenceKind(value as "observed" | "inferred")}>
                <SelectTrigger id="profile-evidence" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="observed">直接观察</SelectItem>
                  <SelectItem value="inferred">推断</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1 text-xs">
              <label htmlFor="profile-summary">来源摘要</label>
              <Textarea
                id="profile-summary"
                required
                maxLength={300}
                value={summary}
                onChange={(event) => setSummary(event.target.value)}
              />
            </div>
            <div className="space-y-1 text-xs">
              <label htmlFor="profile-query">查询语句（可选）</label>
              <Input
                id="profile-query"
                maxLength={200}
                value={query}
                onChange={(event) => setQuery(event.target.value)}
              />
            </div>
            <div className="space-y-1 text-xs">
              <label htmlFor="profile-node">探索证据节点 ID（可选）</label>
              <Input
                id="profile-node"
                type="number"
                min={1}
                value={nodeId}
                onChange={(event) => setNodeId(event.target.value)}
              />
            </div>
            <div className="space-y-1 text-xs">
              <label htmlFor="profile-confidence">置信度：{confidence}%</label>
              <Input
                id="profile-confidence"
                type="range"
                min={0}
                max={100}
                value={confidence}
                onChange={(event) => setConfidence(Number(event.target.value))}
              />
            </div>
          </form>
          <SheetFooter>
            <Button
              type="submit"
              form="target-profile-form"
              disabled={saving || !scopeId || !label.trim() || !summary.trim()}
            >
              保存观察
            </Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>
    </div>
  );
}
