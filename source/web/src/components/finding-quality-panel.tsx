"use client";

import * as React from "react";

import { toast } from "sonner";

import { StatusBadge } from "@/components/status-badge";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { api } from "@/lib/api";
import type { FindingQuality, FindingQualityStatus } from "@/lib/types";

const QUALITY_STATUSES: FindingQualityStatus[] = ["NEEDS_MORE", "PASS", "DOWNGRADE", "DUPLICATE_RISK", "BLOCKED"];

export function FindingQualityPanel({
  findingId,
  quality,
  inherited,
  onChanged,
}: {
  findingId: string;
  quality?: FindingQuality;
  inherited: boolean;
  onChanged: () => void;
}) {
  const [admin, setAdmin] = React.useState(false);
  const [saving, setSaving] = React.useState(false);
  const [draft, setDraft] = React.useState<FindingQuality | null>(quality ?? null);

  React.useEffect(() => {
    setDraft(quality ?? null);
  }, [quality]);

  React.useEffect(() => {
    let alive = true;
    api
      .authMe()
      .then((identity) => alive && setAdmin(identity.role === "admin"))
      .catch(() => undefined);
    return () => {
      alive = false;
    };
  }, []);

  const save = async () => {
    if (!draft) return;
    setSaving(true);
    try {
      await api.updateFindingQuality(findingId, draft);
      toast.success("质量审核已保存");
      onChanged();
    } catch (error) {
      toast.error(`质量审核失败：${(error as Error).message}`);
    } finally {
      setSaving(false);
    }
  };

  if (!quality || !draft) return null;
  const set = <K extends keyof FindingQuality>(key: K, value: FindingQuality[K]) =>
    setDraft((current) => (current ? { ...current, [key]: value } : current));

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center justify-between gap-2 text-sm">
          漏洞质量门禁
          <StatusBadge domain="audit" value={quality.quality_status} />
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {quality.blocking_reason && (
          <Alert>
            <AlertDescription>{quality.blocking_reason}</AlertDescription>
          </Alert>
        )}
        {quality.quality_status !== "PASS" && (
          <p className="text-muted-foreground text-xs">质量状态通过人工审核前，不允许生成正式报告。</p>
        )}
        {quality.reviewer && (
          <p className="text-muted-foreground text-xs">
            审核人：{quality.reviewer}
            {quality.reviewed_at ? ` · ${new Date(quality.reviewed_at).toLocaleString("zh-CN")}` : ""}
          </p>
        )}
        {!admin || inherited ? (
          <div className="grid gap-2 text-xs sm:grid-cols-2">
            <p>
              复现：{quality.success_count}/{quality.reproduction_count}
            </p>
            <p>证据完整性：{quality.evidence_integrity || "未标记"}</p>
            <p className="break-all">基线：{quality.baseline_evidence || "未记录"}</p>
            <p className="break-all">证明：{quality.proof_evidence || "未记录"}</p>
            <p className="break-all">复核：{quality.verification_evidence || "未记录"}</p>
          </div>
        ) : (
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1">
              <Label>质量状态</Label>
              <Select
                value={draft.quality_status}
                onValueChange={(value) => set("quality_status", value as FindingQualityStatus)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {QUALITY_STATUSES.map((status) => (
                    <SelectItem key={status} value={status}>
                      {status}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <Label>漏洞类型</Label>
              <Input
                value={draft.vulnerability_type}
                onChange={(event) => set("vulnerability_type", event.target.value)}
              />
            </div>
            <div className="space-y-1">
              <Label>类型专属验证方式</Label>
              <Input
                value={draft.verification_type}
                onChange={(event) => set("verification_type", event.target.value)}
              />
            </div>
            <div className="grid grid-cols-2 gap-2">
              <div className="space-y-1">
                <Label>复现次数</Label>
                <Input
                  type="number"
                  min={0}
                  max={5}
                  value={draft.reproduction_count}
                  onChange={(event) => set("reproduction_count", Number(event.target.value))}
                />
              </div>
              <div className="space-y-1">
                <Label>成功次数</Label>
                <Input
                  type="number"
                  min={0}
                  max={5}
                  value={draft.success_count}
                  onChange={(event) => set("success_count", Number(event.target.value))}
                />
              </div>
            </div>
            {(
              [
                ["baseline_evidence", "基线/对照证据"],
                ["proof_evidence", "漏洞证明证据"],
                ["verification_evidence", "复核证据"],
                ["verification_rationale", "缺少对照的原因或影响验证依据"],
                ["duplicate_risk", "重复风险说明"],
                ["blocking_reason", "阻塞/补充材料说明"],
              ] as const
            ).map(([key, label]) => (
              <div key={key} className="space-y-1">
                <Label htmlFor={`${findingId}-${key}`}>{label}</Label>
                <Textarea
                  id={`${findingId}-${key}`}
                  rows={2}
                  value={draft[key]}
                  onChange={(event) => set(key, event.target.value)}
                />
              </div>
            ))}
            <div className="space-y-1">
              <Label>证据完整性</Label>
              <Select
                value={draft.evidence_integrity || "unverified"}
                onValueChange={(value) => set("evidence_integrity", value)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="unverified">未核验</SelectItem>
                  <SelectItem value="verified">已人工核验</SelectItem>
                  <SelectItem value="incomplete">不完整</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="flex items-end">
              <Button className="w-full" disabled={saving} onClick={() => void save()}>
                {saving ? "保存中…" : "保存审核结果"}
              </Button>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
