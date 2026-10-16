"use client";

import * as React from "react";

import {
  Loader2Icon,
  PlugZapIcon,
  PlusIcon,
  RefreshCwIcon,
  RotateCcwIcon,
  SaveIcon,
  StarIcon,
  Trash2Icon,
  ZapIcon,
} from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { api } from "@/lib/api";
import type { LLMPoolMember, LLMPoolStatus, LLMProfile, LLMRetryOverride } from "@/lib/types";
import { cn } from "@/lib/utils";

import { ProfileRetryFields, RetryPolicyPanel, ZERO_OVERRIDE } from "./_components/retry";

// 思考开关(thinking.type)与思考强度(reasoning_effort)是两个【互相独立】的字段，
// 各自单独设置——有些接口没有 thinking 字段、只靠强度参数就能激活思考，故需解耦。
// 存库空字符串 = 该字段【不发送】；Radix Select 不接受空 value，故 UI 用 "none"
// 哨兵表示不发送，存取时与 "" 互转（NONE / fromStore / toStore）。
const NONE = "none";
const fromStore = (v?: string) => (v ? v : NONE);
const toStore = (v: string) => (v === NONE ? "" : v);
const THINKING_TYPES: { value: string; label: string }[] = [
  { value: NONE, label: "不发送（默认）" },
  { value: "disabled", label: "关闭" },
  { value: "enabled", label: "开启" },
];
// 输出上限用哪个请求字段名（仅 openai 格式有意义）。NONE ↔ "" 走同一套哨兵转换。
const MAX_TOKENS_FIELDS: { value: string; label: string }[] = [
  { value: NONE, label: "max_tokens（默认）" },
  { value: "max_completion_tokens", label: "max_completion_tokens" },
];
// 另外两种格式各自定死了字段名，选项对它们无意义，说明文案里直接讲清楚。
const MAX_TOKENS_FIELD_HINTS: Record<string, string> = {
  openai:
    "上限发哪个键。max_tokens 是默认，绝大多数兼容网关只认它；OpenAI 官方推理模型（o 系列 / GPT-5）反过来只认 max_completion_tokens，收到 max_tokens 会直接报 unsupported_parameter。",
  anthropic: "仅 openai 格式可选。Anthropic 的字段名固定为 max_tokens。",
  "openai-responses": "仅 openai 格式可选。Responses API 的字段名固定为 max_output_tokens。",
};
const EFFORT_LEVELS: { value: string; label: string }[] = [
  { value: NONE, label: "不发送（默认）" },
  { value: "low", label: "low" },
  { value: "medium", label: "medium" },
  { value: "high", label: "high" },
  { value: "xhigh", label: "xhigh" },
  { value: "max", label: "max" },
];

function cooldownText(secs: number) {
  if (secs <= 0) return "";
  if (secs < 60) return `${secs}s`;
  return `${Math.ceil(secs / 60)}min`;
}

// 一个配置在卡片上显示的「是否正常」。没填 Key 的配置根本发不出请求，比熔断更该先说；
// 其余状态来自轮询的熔断记录（轮询关着时不会产生新记录，此时「正常」= 没有已知故障）。
type Health = { label: string; cls: string; hint?: string };
function healthOf(p: LLMProfile, m?: LLMPoolMember): Health {
  if (!p.api_key_hint) {
    return {
      label: "未配置 Key",
      cls: "border-muted-foreground/40 text-muted-foreground",
      hint: "未填 API Key，无法调用",
    };
  }
  if (m?.state === "tripped") {
    return {
      label: m.cooldown_secs > 0 ? `已熔断 · ${cooldownText(m.cooldown_secs)}` : "已熔断",
      cls: "border-destructive/50 text-destructive",
      hint: m.last_error,
    };
  }
  if (m?.state === "degraded") {
    return {
      label: `异常 · 失败 ${m.fails} 次`,
      cls: "border-amber-500/50 text-amber-600 dark:text-amber-400",
      hint: m.last_error,
    };
  }
  return { label: "正常", cls: "border-emerald-500/50 text-emerald-600 dark:text-emerald-400" };
}

// ─────────────────────────────────────────────────────────────────────────────
// 轮询配置抽屉
// ─────────────────────────────────────────────────────────────────────────────

function PoolSheet({
  open,
  onOpenChange,
  pool,
  onReload,
}: {
  open: boolean;
  onOpenChange: (o: boolean) => void;
  pool: LLMPoolStatus | null;
  onReload: () => Promise<void>;
}) {
  const [busy, setBusy] = React.useState(false);

  // 冷却倒计时是后端算出的剩余秒数——抽屉开着且有配置不正常时才定时拉，让它走起来。
  React.useEffect(() => {
    if (!open || !pool?.enabled || !pool.chain.some((m) => m.state !== "ok")) return;
    const t = setInterval(() => void onReload(), 10_000);
    return () => clearInterval(t);
  }, [open, pool, onReload]);

  async function toggle(patch: { llm_pool_enabled?: boolean; llm_pool_bind_fallback?: boolean }) {
    if (busy) return;
    setBusy(true);
    try {
      await api.setSettings(patch);
      await onReload();
      if (patch.llm_pool_enabled !== undefined) {
        toast.success(patch.llm_pool_enabled ? "已开启 LLM 轮询" : "已关闭 LLM 轮询");
      } else {
        toast.success("已更新兜底设置");
      }
    } catch (e) {
      toast.error(`设置失败：${(e as Error).message}`);
    } finally {
      setBusy(false);
    }
  }

  async function recover(id?: string) {
    try {
      await api.resetLLMPool(id);
      await onReload();
      toast.success(id ? "已恢复该配置" : "已恢复全部配置");
    } catch (e) {
      toast.error(`恢复失败：${(e as Error).message}`);
    }
  }

  const enabled = pool?.enabled ?? false;
  const chain = pool?.chain ?? [];
  // 参与轮询的成员（排除被标记「不参与轮询」的），顺序即后端实际的尝试顺序。
  const inChain = chain.filter((m) => m.active || !m.excluded);
  const tripped = chain.filter((m) => m.state === "tripped");

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="flex flex-col gap-0 p-0 data-[side=right]:sm:max-w-lg">
        <SheetHeader className="px-4">
          <SheetTitle className="flex items-center gap-2">
            <ZapIcon className="size-4" /> LLM 轮询 · 故障转移
          </SheetTitle>
          <SheetDescription>
            开启后，<b>未指定模型</b>的 Agent 在当前配置不可用（余额不足 / Key 失效 / 限流 /
            服务异常）时自动切到下一个配置。
          </SheetDescription>
        </SheetHeader>

        <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-4 pb-6">
          <div className="flex items-center justify-between gap-4 rounded-lg border p-3">
            <div className="grid gap-0.5">
              <Label className="text-sm">启用轮询</Label>
              <p className="text-muted-foreground text-xs">默认关闭。关闭时始终只用激活配置，失败即失败。</p>
            </div>
            <Switch
              checked={enabled}
              disabled={busy}
              onCheckedChange={(v) => void toggle({ llm_pool_enabled: v })}
              aria-label="LLM 轮询开关"
            />
          </div>

          {enabled && (
            <>
              <div className="flex items-center justify-between gap-4 rounded-lg border p-3">
                <div className="grid gap-0.5">
                  <Label className="text-sm">指定模型失败时也兜底</Label>
                  <p className="text-muted-foreground text-xs">
                    默认关闭：Agent 或任务指定了某个配置就只用它，失败即失败（不会悄悄换成别的模型）。
                    开启后，指定的配置失败时也会回落到下面的轮询链。
                  </p>
                </div>
                <Switch
                  checked={pool?.bind_fallback ?? false}
                  disabled={busy}
                  onCheckedChange={(v) => void toggle({ llm_pool_bind_fallback: v })}
                  aria-label="绑定配置失败兜底开关"
                />
              </div>

              <Separator />

              <div className="grid gap-2">
                <div className="flex items-center justify-between">
                  <Label className="text-sm">轮询顺序</Label>
                  {tripped.length > 0 && (
                    <Button size="sm" variant="ghost" onClick={() => void recover()}>
                      <RotateCcwIcon /> 全部恢复
                    </Button>
                  )}
                </div>
                {inChain.length < 2 && (
                  <p className="text-muted-foreground text-xs">
                    当前只有 {inChain.length} 个可用配置，轮询不会生效——至少需要 2 个已填 API Key 且参与轮询的配置。
                  </p>
                )}
                {chain.map((m) => {
                  const excluded = m.excluded && !m.active;
                  const order = excluded ? null : inChain.findIndex((x) => x.profile_id === m.profile_id) + 1;
                  return (
                    <div
                      key={m.profile_id}
                      className={cn(
                        "grid gap-1 rounded-lg border p-2.5 text-sm",
                        excluded && "opacity-55",
                        m.state === "tripped" && "border-destructive/40",
                      )}
                    >
                      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                        <span className="w-5 shrink-0 text-center font-mono text-muted-foreground text-xs">
                          {order ?? "—"}
                        </span>
                        <span className="font-medium">{m.name}</span>
                        {m.active && (
                          <Badge variant="outline" className="border-amber-400/50 text-amber-500">
                            激活
                          </Badge>
                        )}
                        {excluded && <Badge variant="outline">不参与轮询</Badge>}
                        <div className="ml-auto flex items-center gap-2">
                          {m.state === "tripped" && m.cooldown_secs > 0 && (
                            <span className="text-muted-foreground text-xs">冷却 {cooldownText(m.cooldown_secs)}</span>
                          )}
                          {m.state === "degraded" && (
                            <span className="text-muted-foreground text-xs">连续失败 {m.fails} 次</span>
                          )}
                          {m.state !== "ok" && (
                            <Button
                              size="icon"
                              variant="ghost"
                              className="size-7"
                              aria-label="立即恢复"
                              title="立即恢复：清除熔断，下次调用重试该配置"
                              onClick={() => void recover(m.profile_id)}
                            >
                              <RotateCcwIcon className="size-3.5" />
                            </Button>
                          )}
                        </div>
                      </div>
                      <div className="flex flex-wrap items-center gap-x-3 pl-7 text-muted-foreground text-xs">
                        <code className="truncate font-mono">{m.model}</code>
                        {!m.active && <span>优先级 {m.priority}</span>}
                      </div>
                      {m.last_error && (
                        <p className="truncate pl-7 font-mono text-muted-foreground text-xs" title={m.last_error}>
                          {m.last_error}
                        </p>
                      )}
                    </div>
                  );
                })}
                {chain.length === 0 && (
                  <div className="rounded-lg border border-dashed p-4 text-center text-muted-foreground text-sm">
                    暂无配置
                  </div>
                )}
              </div>

              <div className="rounded-lg border border-dashed p-3 text-muted-foreground text-xs leading-relaxed">
                激活配置恒为第 1 顺位，其余按优先级从高到低（在各配置里设置）。某个配置失败后进入冷却 （60s → 5min →
                30min），冷却期内被跳过，恢复后自动切回。上下文窗口装不下当前请求的配置会被跳过。 指定了模型的 Agent
                与任务默认不参与轮询。
              </div>
            </>
          )}
        </div>
      </SheetContent>
    </Sheet>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// 模型配置编辑（新建 / 编辑共用同一套表单）
// ─────────────────────────────────────────────────────────────────────────────

function ConfigurationSection({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="yunkun-configuration-section">
      <h3 className="text-sm font-medium">{title}</h3>
      <div className="min-w-0 space-y-4">{children}</div>
    </section>
  );
}

function ProfileEditor({
  profile,
  onSaved,
  onDirtyChange,
  onBusyChange,
}: {
  profile: LLMProfile | null; // null = 新建
  onSaved: (id: string) => void;
  onDirtyChange: (dirty: boolean) => void;
  onBusyChange: (busy: boolean) => void;
}) {
  const isNew = !profile;
  const [name, setName] = React.useState(profile?.name ?? "");
  const [format, setFormat] = React.useState<"anthropic" | "openai" | "openai-responses">(
    profile?.format === "openai" || profile?.format === "openai-responses" ? profile.format : "anthropic",
  );
  const [model, setModel] = React.useState(profile?.model ?? "");
  const [baseUrl, setBaseUrl] = React.useState(profile?.base_url ?? "");
  const [proxy, setProxy] = React.useState(profile?.proxy ?? "");
  const [apiKey, setApiKey] = React.useState("");
  const keyHint = profile?.api_key_hint ?? "";
  const [rps, setRps] = React.useState(String(profile?.rate_per_second ?? 0));
  const [rpm, setRpm] = React.useState(String(profile?.rate_per_minute ?? 0));
  const [cw, setCw] = React.useState(String(profile?.context_window_k ?? 0));
  const [cwConfirmed, setCwConfirmed] = React.useState(profile?.context_window_confirmed ?? false);
  const [thinkingType, setThinkingType] = React.useState(fromStore(profile?.thinking_type));
  const [effort, setEffort] = React.useState(fromStore(profile?.reasoning_effort));
  const [priority, setPriority] = React.useState(String(profile?.priority ?? 0));
  const [poolExclude, setPoolExclude] = React.useState(profile?.pool_exclude ?? false);
  const [streaming, setStreaming] = React.useState(profile?.streaming ?? true);
  const [maxTokens, setMaxTokens] = React.useState(String(profile?.max_tokens ?? 0));
  const [maxTokensField, setMaxTokensField] = React.useState(fromStore(profile?.max_tokens_field));
  const [sessionHeaderKey, setSessionHeaderKey] = React.useState(profile?.session_header_key ?? "");
  const [retry, setRetry] = React.useState<LLMRetryOverride>(profile?.retry ?? ZERO_OVERRIDE);
  const [testing, setTesting] = React.useState(false);
  const [saving, setSaving] = React.useState(false);
  const [models, setModels] = React.useState<string[]>([]);
  const [loadingModels, setLoadingModels] = React.useState(false);
  const [modelsOpen, setModelsOpen] = React.useState(false);

  // The parent keys this editor by profile ID; background list refreshes never reset a draft.
  const draft = JSON.stringify([
    name,
    format,
    model,
    baseUrl,
    proxy,
    apiKey,
    rps,
    rpm,
    cw,
    cwConfirmed,
    thinkingType,
    effort,
    priority,
    poolExclude,
    streaming,
    maxTokens,
    maxTokensField,
    sessionHeaderKey,
    retry,
  ]);
  const baseline = React.useRef(draft);
  const dirty = draft !== baseline.current;
  const busy = testing || saving || loadingModels;

  React.useEffect(() => {
    onDirtyChange(dirty);
    return () => onDirtyChange(false);
  }, [dirty, onDirtyChange]);
  React.useEffect(() => {
    onBusyChange(busy);
    return () => onBusyChange(false);
  }, [busy, onBusyChange]);
  React.useEffect(() => {
    if (!dirty) return;
    const beforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    const beforeNavigate = (event: MouseEvent) => {
      const anchor = event.target instanceof Element ? event.target.closest("a[href]") : null;
      if (!(anchor instanceof HTMLAnchorElement) || anchor.target === "_blank" || event.defaultPrevented) return;
      const target = new URL(anchor.href);
      if (
        target.origin === location.origin &&
        target.pathname !== location.pathname &&
        !window.confirm("当前配置有未保存的修改，确定离开？")
      ) {
        event.preventDefault();
        event.stopPropagation();
      }
    };
    const beforeSearchNavigate = (event: Event) => {
      if (!window.confirm("当前配置有未保存的修改，确定离开？")) event.preventDefault();
    };
    window.addEventListener("beforeunload", beforeUnload);
    document.addEventListener("click", beforeNavigate, true);
    document.addEventListener("yunkun:before-navigate", beforeSearchNavigate);
    return () => {
      window.removeEventListener("beforeunload", beforeUnload);
      document.removeEventListener("click", beforeNavigate, true);
      document.removeEventListener("yunkun:before-navigate", beforeSearchNavigate);
    };
  }, [dirty]);

  const profileId = profile ? Number(profile.id) : undefined;

  async function loadModels() {
    if (loadingModels) return;
    setLoadingModels(true);
    setModels([]);
    try {
      const r = await api.fetchLLMModels(format, baseUrl, apiKey, proxy, profileId);
      if (r.ok && r.models && r.models.length > 0) {
        setModels(r.models);
        setModelsOpen(true);
        toast.success(`已加载 ${r.models.length} 个模型`);
      } else {
        toast.error(`加载模型失败：${r.error ?? "未获取到模型"}`);
      }
    } catch (e) {
      toast.error(`加载模型出错：${(e as Error).message}`);
    } finally {
      setLoadingModels(false);
    }
  }

  async function testConnection() {
    if (testing) return;
    setTesting(true);
    try {
      // 用配置实际会跑的思考参数来测，这样不支持该字段的模型在这里就失败，
      // 而不是等到跑任务时才炸。传 profile id：Key 输入框留空时用已存的 Key。
      const r = await api.testLLM(
        format,
        model,
        baseUrl,
        apiKey,
        proxy,
        toStore(thinkingType),
        toStore(effort),
        profileId,
        streaming,
        sessionHeaderKey.trim(),
      );
      // 回复内容一并展示：看得见模型确实说了话，才算和会话里跑通是一回事。
      if (r.ok)
        toast.success(`连接成功 · ${r.latency_ms ?? "?"}ms · ${r.model ?? model}`, {
          description: r.reply ? `回复：${r.reply}` : undefined,
        });
      else toast.error(`连接失败：${r.error ?? "未知"}`);
    } catch (e) {
      toast.error(`测试出错：${(e as Error).message}`);
    } finally {
      setTesting(false);
    }
  }

  async function save() {
    if (!name.trim() || !model.trim()) {
      toast.error("请填写名称与模型");
      return;
    }
    if (saving) return;
    setSaving(true);
    try {
      const { id } = await api.saveLLMProfile({
        ...(profile ? { id: Number(profile.id) } : {}),
        name: name.trim(),
        format,
        model: model.trim(),
        base_url: baseUrl.trim(),
        proxy: proxy.trim(),
        api_key: apiKey,
        rate_per_second: Number(rps) || 0,
        rate_per_minute: Number(rpm) || 0,
        context_window_k: Number(cw) || 0,
        context_window_confirmed: cwConfirmed && Number(cw) > 0,
        thinking_type: toStore(thinkingType),
        reasoning_effort: toStore(effort),
        priority: Number(priority) || 0,
        pool_exclude: poolExclude,
        streaming,
        max_tokens: Math.max(0, Number(maxTokens) || 0),
        // 字段名开关只对 openai(Chat Completions) 有意义，其它格式一律回落到默认；
        // 后端也会再做一次同样的归一化，这里只是别让 UI 送出自相矛盾的值。
        max_tokens_field: format === "openai" ? toStore(maxTokensField) : "",
        session_header_key: sessionHeaderKey.trim(),
        retry,
      });
      if (isNew) toast.success(`已新建：${name.trim()}`);
      else toast.success(profile?.is_default ? "已保存，激活配置即时生效，无需重启" : "已保存");
      onSaved(String(id));
    } catch (e) {
      toast.error(`保存失败：${(e as Error).message}`);
    } finally {
      setSaving(false);
    }
  }

  return (
    <section className="yunkun-profile-editor min-w-0" aria-label="模型配置编辑">
      <header className="flex flex-wrap items-center justify-between gap-3 border-b pb-5">
        <h2 className="flex min-w-0 flex-wrap items-center gap-2 text-base font-semibold">
          {isNew ? "新建模型配置" : `编辑：${profile?.name}`}
          {profile?.is_default && (
            <Badge variant="outline" className="border-amber-400/50 text-amber-500">
              激活中
            </Badge>
          )}
        </h2>
        {dirty && <span className="text-xs text-amber-700 dark:text-amber-400">未保存</span>}
      </header>

      <fieldset disabled={busy} className="min-w-0">
        <ConfigurationSection title="基础配置">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-2">
              <Label htmlFor="p-name">名称</Label>
              <Input
                id="p-name"
                placeholder="例如：OpenAI 生产"
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <Label>格式</Label>
              <Select value={format} onValueChange={(v) => setFormat(v as "anthropic" | "openai" | "openai-responses")}>
                <SelectTrigger>
                  <SelectValue placeholder="选择格式" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="anthropic">Anthropic</SelectItem>
                  <SelectItem value="openai">OpenAI (Chat Completions)</SelectItem>
                  <SelectItem value="openai-responses">OpenAI (Responses API)</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="grid gap-2">
            <Label htmlFor="p-model">模型</Label>
            <div className="flex gap-2">
              <Input
                id="p-model"
                className="font-mono"
                placeholder="claude-opus-4-8"
                value={model}
                onChange={(e) => {
                  setModel(e.target.value);
                  setCwConfirmed(false);
                }}
              />
              <Popover open={modelsOpen} onOpenChange={setModelsOpen} modal>
                <PopoverTrigger asChild>
                  <Button
                    type="button"
                    variant="outline"
                    size="icon"
                    className="shrink-0"
                    disabled={loadingModels}
                    onClick={loadModels}
                    title="从 API 加载可用模型"
                  >
                    {loadingModels ? <Loader2Icon className="animate-spin" /> : <RefreshCwIcon />}
                  </Button>
                </PopoverTrigger>
                {models.length > 0 && (
                  <PopoverContent className="max-h-72 w-72 gap-0 overflow-y-auto overscroll-contain p-1" align="end">
                    {models.map((m) => (
                      <button
                        key={m}
                        type="button"
                        className="w-full shrink-0 rounded-md px-2 py-1.5 text-left font-mono text-xs hover:bg-accent hover:text-accent-foreground"
                        onClick={() => {
                          setModel(m);
                          setModelsOpen(false);
                        }}
                      >
                        {m}
                      </button>
                    ))}
                  </PopoverContent>
                )}
              </Popover>
            </div>
          </div>
        </ConfigurationSection>
        <ConfigurationSection title="连接设置">
          <div className="grid gap-2">
            <Label htmlFor="p-base-url">Base URL（可选）</Label>
            <Input
              id="p-base-url"
              className="font-mono"
              placeholder="https://api.openai.com/v1"
              value={baseUrl}
              onChange={(e) => setBaseUrl(e.target.value)}
            />
          </div>

          <div className="grid gap-2">
            <Label htmlFor="p-proxy">代理（可选）</Label>
            <Input
              id="p-proxy"
              className="font-mono"
              placeholder="socks5://user:pass@127.0.0.1:1080 · http://127.0.0.1:8080"
              value={proxy}
              onChange={(e) => setProxy(e.target.value)}
            />
            <p className="text-muted-foreground text-xs">
              仅 LLM 出站请求走此代理，支持 http/https/socks5，可带账号密码（如
              socks5://user:pass@host:port，密码含特殊字符需 URL 编码）；留空表示不使用代理（直连）。
            </p>
          </div>

          <div className="grid gap-2">
            <Label htmlFor="p-session-header">自定义会话头（可选）</Label>
            <Input
              id="p-session-header"
              className="font-mono"
              placeholder="如 x-session-id（留空=不发送）"
              value={sessionHeaderKey}
              onChange={(e) => setSessionHeaderKey(e.target.value)}
            />
            <p className="text-muted-foreground text-xs">
              填写头名后，每次请求都会带上这个 HTTP 头，头值自动填为 <b>当前会话的 session id</b>（chat 会话如
              conv-12、worker 如 exp3-worker-i87）。用于按 session-id 头做提示缓存 /
              粘性路由的网关；同一会话多轮稳定、不同会话互不相同。留空则不发送。
            </p>
          </div>

          <div className="grid gap-2">
            <Label htmlFor="p-api-key">API Key</Label>
            <Input
              id="p-api-key"
              type="password"
              placeholder={keyHint ? `已设置（${keyHint}），留空保持不变` : "sk-…"}
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
            />
          </div>
        </ConfigurationSection>
        <ConfigurationSection title="限速与上下文">
          <div className="grid gap-4 sm:grid-cols-3">
            <div className="grid gap-2">
              <Label htmlFor="p-rps">每秒限速</Label>
              <Input id="p-rps" type="number" min={0} value={rps} onChange={(e) => setRps(e.target.value)} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="p-rpm">每分钟限速</Label>
              <Input id="p-rpm" type="number" min={0} value={rpm} onChange={(e) => setRpm(e.target.value)} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="p-cw">上下文窗口(K)</Label>
              <Input
                id="p-cw"
                type="number"
                min={0}
                max={1000}
                value={cw}
                onChange={(e) => {
                  setCw(e.target.value);
                  setCwConfirmed(false);
                }}
                placeholder="200"
              />
            </div>
          </div>
          <p className="-mt-2 text-muted-foreground text-xs">
            限速 0 = 不限，全 Agent 共享。上下文窗口单位 K（千 token），上限 1000（即 1M）。
            未核实容量时，请求预算与历史压缩窗口按本地策略最多 64K 估算；输出上限超过本次剩余容量时会自动降低。
          </p>
          <div className="flex items-center justify-between gap-4">
            <Label htmlFor="p-cw-confirmed">已核实该模型的上下文容量</Label>
            <Switch
              id="p-cw-confirmed"
              checked={cwConfirmed}
              disabled={Number(cw) <= 0}
              onCheckedChange={setCwConfirmed}
            />
          </div>
        </ConfigurationSection>
        <ConfigurationSection title="轮询配置">
          <div className="grid gap-3">
            <div className="flex items-center justify-between gap-4">
              <div className="grid gap-0.5">
                <Label htmlFor="p-priority" className="text-sm">
                  轮询优先级
                </Label>
                <p className="text-muted-foreground text-xs">
                  数字越大越先被选中；激活配置恒为第 1 顺位，与本值无关。相同优先级的配置会轮流打头，天然分摊额度。
                </p>
              </div>
              <Input
                id="p-priority"
                type="number"
                className="w-24 shrink-0"
                value={priority}
                onChange={(e) => setPriority(e.target.value)}
              />
            </div>
            <div className="flex items-center justify-between gap-4 border-t pt-3">
              <div className="grid gap-0.5">
                <Label className="text-sm">不参与轮询</Label>
                <p className="text-muted-foreground text-xs">
                  开启后不会被当作故障转移目标（仍可被 Agent / 任务显式指定使用）。 适合「只给某个 Agent
                  专用、不希望别人失败时烧掉」的昂贵配置。
                </p>
              </div>
              <Switch checked={poolExclude} onCheckedChange={setPoolExclude} aria-label="不参与轮询" />
            </div>
            <div className="flex items-center justify-between gap-4 border-t pt-3">
              <div className="grid gap-0.5">
                <Label className="text-sm">流式输出 · streaming</Label>
                <p className="text-muted-foreground text-xs">
                  开启（默认）走流式 SSE，有运行中实时进度与实时 token 计数。 关闭则走真·非流式（stream:false，
                  一次性返回完整响应）——可绕开部分网关糟糕的 SSE 实现（空帧 / 思考字段丢帧），
                  代价是失去运行中的实时进度。
                </p>
              </div>
              <Switch checked={streaming} onCheckedChange={setStreaming} aria-label="流式输出" />
            </div>
          </div>
        </ConfigurationSection>
        <ConfigurationSection title="输出设置">
          <div className="grid gap-3">
            <div className="flex items-center justify-between gap-4">
              <div className="grid gap-0.5">
                <Label htmlFor="p-max-tokens" className="text-sm">
                  输出上限 · max tokens
                </Label>
                <p className="text-muted-foreground text-xs">
                  单次回复最多生成多少 token，随每次请求发出。0（默认）使用本地输出限额，通常为 8192；
                  小窗口会进一步降低默认值。上下文窗口用于总输入与输出预算及历史压缩。
                  设太小会让推理模型在思考阶段就被截断，一个字答案都出不来。
                </p>
              </div>
              <Input
                id="p-max-tokens"
                type="number"
                min={0}
                className="w-28 shrink-0"
                value={maxTokens}
                onChange={(e) => setMaxTokens(e.target.value)}
                placeholder="0"
              />
            </div>
            <div className="flex items-center justify-between gap-4 border-t pt-3">
              <div className="grid gap-0.5">
                <Label className="text-sm">上限字段名</Label>
                <p className="text-muted-foreground text-xs">{MAX_TOKENS_FIELD_HINTS[format]}</p>
              </div>
              <Select
                value={format === "openai" ? maxTokensField : NONE}
                onValueChange={setMaxTokensField}
                disabled={format !== "openai"}
              >
                <SelectTrigger className="w-56 shrink-0">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {MAX_TOKENS_FIELDS.map((o) => (
                    <SelectItem key={o.value} value={o.value}>
                      {o.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
        </ConfigurationSection>
        <ConfigurationSection title="思考设置">
          <div className="grid gap-3">
            <div className="flex items-center justify-between gap-4">
              <div className="grid gap-0.5">
                <Label className="text-sm">思考开关 · thinking.type</Label>
                <p className="text-muted-foreground text-xs">
                  控制是否发送 thinking 字段。不发送=不带该字段（兼容 MiniMax 等不支持 的模型）；关闭=发
                  disabled；开启=发 enabled。与下面的强度互相独立。
                </p>
              </div>
              <Select value={thinkingType} onValueChange={setThinkingType}>
                <SelectTrigger className="w-32 shrink-0">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {THINKING_TYPES.map((o) => (
                    <SelectItem key={o.value} value={o.value}>
                      {o.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="flex items-center justify-between gap-4 border-t pt-3">
              <div className="grid gap-0.5">
                <Label className="text-sm">思考强度 · reasoning_effort</Label>
                <p className="text-muted-foreground text-xs">
                  独立的强度档位（OpenAI reasoning_effort / Anthropic output_config.effort）。 有些接口没有 thinking
                  字段、只靠强度即可激活思考，故可单独设置、不发送思考开关。
                </p>
              </div>
              <Select value={effort} onValueChange={setEffort}>
                <SelectTrigger className="w-32 shrink-0">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {EFFORT_LEVELS.map((o) => (
                    <SelectItem key={o.value} value={o.value}>
                      {o.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
        </ConfigurationSection>
        <ConfigurationSection title="重试与退避">
          <ProfileRetryFields value={retry} onChange={setRetry} />
        </ConfigurationSection>
      </fieldset>

      <div className="flex flex-wrap justify-end gap-2 border-t py-4">
        <Button variant="outline" onClick={testConnection} disabled={busy}>
          {testing ? <Loader2Icon className="animate-spin" /> : <PlugZapIcon />}
          {testing ? "测试中…" : "测试连接"}
        </Button>
        <Button onClick={save} disabled={busy}>
          {saving && <Loader2Icon className="animate-spin" />}
          {!saving && (isNew ? <PlusIcon /> : <SaveIcon />)}
          {isNew ? "新建" : "保存"}
        </Button>
      </div>
    </section>
  );
}

// ─────────────────────────────────────────────────────────────────────────────

export default function LLMPage() {
  const [profiles, setProfiles] = React.useState<LLMProfile[]>([]);
  const [pool, setPool] = React.useState<LLMPoolStatus | null>(null);
  const [poolOpen, setPoolOpen] = React.useState(false);
  const [editOpen, setEditOpen] = React.useState(false);
  const [editing, setEditing] = React.useState<LLMProfile | null>(null);
  const [tab, setTab] = React.useState("profiles");
  const dirtyRef = React.useRef(false);
  const busyRef = React.useRef(false);
  const onDirtyChange = React.useCallback((dirty: boolean) => {
    dirtyRef.current = dirty;
  }, []);
  const onBusyChange = React.useCallback((busy: boolean) => {
    busyRef.current = busy;
  }, []);
  function canLeaveEditor() {
    if (busyRef.current) {
      toast.info("请等待当前操作完成");
      return false;
    }
    return !dirtyRef.current || window.confirm("当前配置有未保存的修改，确定放弃？");
  }
  function openEditor(p: LLMProfile | null) {
    if (editOpen && (editing?.id ?? null) === (p?.id ?? null)) return;
    if (!canLeaveEditor()) return;
    dirtyRef.current = false;
    setEditing(p);
    setEditOpen(true);
    setTab("profiles");
  }

  const loadPool = React.useCallback(async () => {
    try {
      setPool(await api.llmPool());
    } catch {
      /* ignore */
    }
  }, []);

  const load = React.useCallback(async () => {
    try {
      setProfiles(await api.llmProfiles());
    } catch {
      /* ignore */
    }
    await loadPool();
  }, [loadPool]);

  React.useEffect(() => {
    void load();
  }, [load]);

  // 健康徽章按 profile id 取轮询状态。
  const health = React.useMemo(() => {
    const m = new Map<string, LLMPoolMember>();
    for (const c of pool?.chain ?? []) m.set(c.profile_id, c);
    return m;
  }, [pool]);

  async function activate(id: string, name: string) {
    try {
      await api.activateLLMProfile(id);
      toast.success(`已激活：${name}`);
      await load();
    } catch (e) {
      toast.error(`激活失败：${(e as Error).message}`);
    }
  }

  async function remove(p: LLMProfile) {
    if (p.is_default) {
      toast.error("无法删除当前激活的配置");
      return;
    }
    if (editOpen && editing?.id === p.id && !canLeaveEditor()) return;
    try {
      await api.deleteLLMProfile(p.id);
      if (editing?.id === p.id) setEditOpen(false);
      toast.success(`已删除：${p.name}`);
      await load();
    } catch (e) {
      toast.error(`删除失败：${(e as Error).message}`);
    }
  }

  const poolOn = pool?.enabled ?? false;

  return (
    <div className="flex flex-1 flex-col gap-4 md:gap-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="font-semibold text-xl tracking-tight">LLM</h1>
          <p className="mt-1 text-muted-foreground text-xs">{profiles.length} 个模型配置</p>
        </div>
        <div className="flex items-center gap-2">
          <Button size="sm" variant="outline" onClick={() => setPoolOpen(true)}>
            <ZapIcon /> 轮询配置
            {poolOn && (
              <Badge variant="outline" className="ml-1 border-emerald-500/50 text-emerald-600 dark:text-emerald-400">
                已开启
              </Badge>
            )}
          </Button>
          <Button size="sm" variant="outline" onClick={() => openEditor(null)}>
            <PlusIcon /> 新建
          </Button>
        </div>
      </div>

      <Tabs
        value={tab}
        onValueChange={(value) => {
          if (value === tab || !canLeaveEditor()) return;
          setEditOpen(false);
          setTab(value);
        }}
        className="flex-1"
      >
        <TabsList>
          <TabsTrigger value="profiles">模型配置</TabsTrigger>
          <TabsTrigger value="retry">重试与退避</TabsTrigger>
        </TabsList>

        <TabsContent value="profiles" className="mt-4">
          <div className="yunkun-llm-workspace">
            <aside className="yunkun-profile-list" aria-label="模型配置列表">
              {profiles.map((p) => {
                const h = healthOf(p, health.get(p.id));
                return (
                  <div
                    key={p.id}
                    className={cn("yunkun-profile-row", editOpen && editing?.id === p.id && "is-selected")}
                  >
                    <button
                      type="button"
                      onClick={() => openEditor(p)}
                      aria-pressed={editOpen && editing?.id === p.id}
                      className="grid w-full gap-2 px-4 pt-4 pb-2 text-left"
                    >
                      <div className="flex min-w-0 items-start gap-2">
                        <StarIcon
                          className={cn(
                            "mt-0.5 size-4 shrink-0",
                            p.is_default ? "fill-amber-400 text-amber-400" : "text-muted-foreground",
                          )}
                        />
                        <div className="min-w-0 flex-1">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="truncate font-medium text-sm">{p.name}</span>
                          </div>
                          <code className="mt-1 block truncate font-mono text-muted-foreground text-xs">{p.model}</code>
                          <div className="mt-2 flex flex-wrap items-center gap-2">
                            <span className="text-[10px] uppercase text-muted-foreground">{p.format}</span>
                            <Badge variant="outline" className={h.cls} title={h.hint}>
                              {h.label}
                            </Badge>
                          </div>
                        </div>
                      </div>

                      <div className="flex flex-wrap gap-x-3 gap-y-0.5 pl-6 text-muted-foreground text-xs">
                        {p.api_key_hint && <span>{p.api_key_hint}</span>}
                        <span>
                          {p.rate_per_second}/s · {p.rate_per_minute}/min
                        </span>
                        {p.proxy && <span>已配置代理</span>}
                        {p.reasoning_effort && (
                          <span>思考 {p.reasoning_effort === "off" ? "关" : p.reasoning_effort}</span>
                        )}
                        {/* 轮询相关的两个字段只在轮询开着时才有意义，关着时不占版面 */}
                        {poolOn &&
                          !p.is_default &&
                          (p.pool_exclude ? <span>不参与轮询</span> : <span>优先级 {p.priority ?? 0}</span>)}
                      </div>
                    </button>
                    <div className="flex gap-2 px-4 pb-4">
                      <Button
                        size="sm"
                        variant="outline"
                        className="flex-1"
                        disabled={p.is_default}
                        onClick={(e) => {
                          e.stopPropagation();
                          void activate(p.id, p.name);
                        }}
                      >
                        {p.is_default ? "已激活" : "设为激活"}
                      </Button>
                      <Button
                        size="icon"
                        variant="outline"
                        aria-label="删除配置"
                        title="删除配置"
                        onClick={(e) => {
                          e.stopPropagation();
                          void remove(p);
                        }}
                      >
                        <Trash2Icon className="text-destructive" />
                      </Button>
                    </div>
                  </div>
                );
              })}
              {profiles.length === 0 && (
                <div className="p-6 text-center text-muted-foreground text-sm">暂无模型配置</div>
              )}
            </aside>
            {editOpen ? (
              <ProfileEditor
                key={editing?.id ?? "new"}
                profile={editing}
                onDirtyChange={onDirtyChange}
                onBusyChange={onBusyChange}
                onSaved={() => {
                  dirtyRef.current = false;
                  setEditOpen(false);
                  void load();
                }}
              />
            ) : (
              <div className="flex min-h-64 items-center justify-center text-sm text-muted-foreground">
                未选择模型配置
              </div>
            )}
          </div>
        </TabsContent>

        <TabsContent value="retry" className="mt-4">
          <RetryPolicyPanel />
        </TabsContent>
      </Tabs>

      <PoolSheet open={poolOpen} onOpenChange={setPoolOpen} pool={pool} onReload={loadPool} />
    </div>
  );
}
