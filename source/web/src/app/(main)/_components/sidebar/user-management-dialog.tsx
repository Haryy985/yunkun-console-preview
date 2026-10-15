"use client";

import * as React from "react";

import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { api } from "@/lib/api";

type User = { id: number; username: string; role: "operator" | "viewer"; disabled: boolean };

export function UserManagementDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const [users, setUsers] = React.useState<User[]>([]);
  const [username, setUsername] = React.useState("");
  const [password, setPassword] = React.useState("");
  const [role, setRole] = React.useState<"operator" | "viewer">("viewer");
  const [busy, setBusy] = React.useState(false);

  const refresh = React.useCallback(() => {
    api.authUsers().then(setUsers).catch((error) => toast.error((error as Error).message));
  }, []);
  React.useEffect(() => { if (open) refresh(); }, [open, refresh]);

  async function create(event: React.FormEvent) {
    event.preventDefault();
    setBusy(true);
    try {
      await api.createAuthUser(username.trim(), password, role);
      setUsername("");
      setPassword("");
      refresh();
    } catch (error) {
      toast.error((error as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function update(user: User, next: Partial<User>, newPassword = "") {
    setBusy(true);
    try {
      await api.updateAuthUser(user.id, next.role ?? user.role, next.disabled ?? user.disabled, newPassword);
      refresh();
    } catch (error) {
      toast.error((error as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader><DialogTitle>用户与权限</DialogTitle></DialogHeader>
        <div className="max-h-64 overflow-auto divide-y">
          {users.map((user) => (
            <div key={user.id} className="flex flex-wrap items-center gap-2 py-2">
              <span className="min-w-24 flex-1 text-sm">{user.username}</span>
              <Select value={user.role} disabled={busy} onValueChange={(value) => void update(user, { role: value as User["role"] })}>
                <SelectTrigger className="w-28" aria-label={`${user.username} 角色`}><SelectValue /></SelectTrigger>
                <SelectContent><SelectItem value="viewer">只读</SelectItem><SelectItem value="operator">操作员</SelectItem></SelectContent>
              </Select>
              <Switch checked={!user.disabled} disabled={busy} aria-label={`${user.username} 启用`} onCheckedChange={(checked) => void update(user, { disabled: !checked })} />
              <Button size="sm" variant="outline" disabled={busy} onClick={() => {
                const next = window.prompt(`为 ${user.username} 设置新密码（至少 8 位）`);
                if (next) void update(user, {}, next);
              }}>重置密码</Button>
            </div>
          ))}
        </div>
        <form onSubmit={(event) => void create(event)} className="grid gap-3 border-t pt-4">
          <Label htmlFor="new-user">新用户</Label>
          <Input id="new-user" value={username} onChange={(event) => setUsername(event.target.value)} placeholder="用户名" required maxLength={64} />
          <Input type="password" value={password} onChange={(event) => setPassword(event.target.value)} placeholder="密码（至少 8 位）" required minLength={8} autoComplete="new-password" />
          <Select value={role} onValueChange={(value) => setRole(value as User["role"])}>
            <SelectTrigger aria-label="新用户角色"><SelectValue /></SelectTrigger>
            <SelectContent><SelectItem value="viewer">只读</SelectItem><SelectItem value="operator">操作员</SelectItem></SelectContent>
          </Select>
          <Button type="submit" disabled={busy}>创建用户</Button>
        </form>
      </DialogContent>
    </Dialog>
  );
}
