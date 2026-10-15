// Run in the existing console origin. Default: inspect only.
globalThis.yunkunBrandMigration = function ({ apply = false } = {}) {
  const pairs = [
    ["artex_chat_send_mode", "yunkun_chat_send_mode"],
    ["artex.chat.collapsed-agents", "yunkun.chat.collapsed-agents"],
    ["artex_task_list_sort", "yunkun_task_list_sort"],
    ["artex_task_list_filters", "yunkun_task_list_filters"],
    ["artex_finding_list_preferences", "yunkun_finding_list_preferences"],
    ["artex_task_findings_sort", "yunkun_task_findings_sort"],
  ];
  const result = [];
  for (const [oldKey, newKey] of pairs) {
    try {
      const oldValue = localStorage.getItem(oldKey);
      const current = localStorage.getItem(newKey);
      if (oldValue === null) {
        result.push({ key: newKey, status: "no-legacy-value" });
        continue;
      }
      if (!apply) {
        result.push({ key: newKey, status: current === null ? "would-migrate" : "would-keep-current" });
        continue;
      }
      if (current === null) {
        localStorage.setItem(newKey, oldValue);
        if (localStorage.getItem(newKey) !== oldValue) {
          throw new Error("storage-write-not-confirmed");
        }
      }
      localStorage.removeItem(oldKey);
      result.push({ key: newKey, status: current === null ? "migrated" : "kept-current" });
    } catch {
      result.push({ key: newKey, status: "failed-source-preserved" });
    }
  }
  if (apply) {
    // Old JWT subjects cannot be migrated by changing their encoded payload.
    for (const key of ["artex_token", "yunkun_token"]) {
      try {
        localStorage.removeItem(key);
        document.cookie = `${key}=; path=/; max-age=0; SameSite=Lax`;
      } catch {
        result.push({ key, status: "auth-cleanup-failed" });
      }
    }
  }
  return result;
};
