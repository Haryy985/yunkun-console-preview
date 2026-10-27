"use client";

import * as React from "react";
import { api } from "@/lib/api";
import type { ProxyNode, ProxyPool } from "@/lib/proxy-pool-types";

export function TaskProxyPicker({
  value,
  nodeValue,
  onChange,
  onNodeChange,
  onValidityChange,
}: {
  value?: number;
  nodeValue?: number;
  onChange: (value?: number) => void;
  onNodeChange: (value?: number) => void;
  onValidityChange: (valid: boolean) => void;
}) {
  const [pools, setPools] = React.useState<ProxyPool[]>([]);
  const [nodes, setNodes] = React.useState<ProxyNode[]>([]);
  const [mode, setMode] = React.useState<"auto" | "manual">(nodeValue ? "manual" : "auto");
  const initialized = React.useRef(false);
  const [error, setError] = React.useState("");
  React.useEffect(() => {
    let active = true;
    Promise.all([api.proxyPools(), api.proxySettings()])
      .then(([poolResponse, settings]) => {
        if (!active) return;
        setPools(poolResponse.pools ?? []);
        if (!initialized.current) {
          initialized.current = true;
          if (settings.enabled && settings.default_pool_id) onChange(settings.default_pool_id);
        }
      })
      .catch((e: Error) => {
        if (active) setError(e.message);
      });
    return () => {
      active = false;
    };
  }, []);
  React.useEffect(() => {
    onValidityChange(!value || mode === "auto" || nodes.some((node) => node.id === nodeValue));
  }, [value, mode, nodes, nodeValue, onValidityChange]);
  React.useEffect(() => {
    let active = true;
    setNodes([]);
    if (!value) {
      onNodeChange(undefined);
      return;
    }
    api
      .proxyNodes(value)
      .then((r) => {
        if (!active) return;
        setNodes((r.nodes ?? []).filter((node) => node.enabled && !node.deleted_at &&
          !node.last_error && (!node.expires_at || new Date(node.expires_at).getTime() > Date.now()) &&
          (!node.runtime_cooldown_until || new Date(node.runtime_cooldown_until).getTime() <= Date.now())));
      })
      .catch(() => { if (active) setNodes([]); });
    return () => { active = false; };
  }, [value, onNodeChange]);
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <label htmlFor="task-proxy-pool" className="font-medium text-sm">任务代理池</label>
        <select
          className="h-8 rounded-md border bg-background px-2 text-xs"
          value={value ? "enabled" : "disabled"}
          onChange={(event) => {
            if (event.target.value === "disabled") {
              onChange(undefined);
              onNodeChange(undefined);
            }
          }}
        >
          <option value="enabled">使用代理</option>
          <option value="disabled">关闭代理</option>
        </select>
      </div>
      <select
        id="task-proxy-pool"
        className="h-9 w-full rounded-md border bg-background px-3 text-sm"
        value={value ?? ""}
        onChange={(e) => {
          onNodeChange(undefined);
          onChange(e.target.value ? Number(e.target.value) : undefined);
        }}
      >
        <option value="">跟随全局出口配置</option>
        {pools
          .filter((p) => p.config.enabled)
          .map((p) => (
            <option key={p.id} value={p.id}>
              {p.config.name}
            </option>
          ))}
      </select>
      {value ? (
        <>
          <div className="flex items-center justify-between gap-3">
            <span className="text-muted-foreground text-xs">节点分配</span>
            <select aria-label="节点分配" className="h-8 rounded-md border bg-background px-2 text-xs" value={mode} onChange={(event) => {
              const next = event.target.value as "auto" | "manual";
              setMode(next);
              if (next === "auto") onNodeChange(undefined);
            }}>
              <option value="auto">自动轮询（默认）</option>
              <option value="manual">手动固定节点</option>
            </select>
          </div>
          {mode === "manual" ? (
            <select
              aria-label="固定代理节点"
              className="h-9 w-full rounded-md border bg-background px-3 text-sm"
              value={nodeValue ?? ""}
              onChange={(event) => onNodeChange(event.target.value ? Number(event.target.value) : undefined)}
            >
              <option value="">选择可用节点</option>
              {nodes.map((node) => <option key={node.id} value={node.id}>{node.address}{node.verified_ip ? ` · ${node.verified_ip}` : ""}</option>)}
            </select>
          ) : null}
        </>
      ) : null}
      {value && mode === "manual" && !nodes.some((node) => node.id === nodeValue) && (
        <p role="alert" className="text-destructive text-xs">请选择当前池的可用固定节点</p>
      )}
      {error && (
        <p role="alert" className="text-destructive text-sm">
          {error}
        </p>
      )}
      {value && <p className="text-amber-700 text-xs dark:text-amber-400">{mode === "auto" ? "自动轮询" : "固定节点"} · 配置模式 · 网络隔离尚未验收</p>}
    </div>
  );
}
