const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const script = fs.readFileSync(path.join(__dirname, "browser-preferences.js"), "utf8");

function fixture(failWrite = false) {
  const values = new Map([
    ["artex_task_list_sort", '{"field":"created_at"}'],
    ["artex_task_list_filters", "old"],
    ["yunkun_task_list_filters", "current"],
    ["artex_token", "old-token"],
    ["yunkun_token", "current-token"],
  ]);
  const cookies = [];
  const document = {};
  Object.defineProperty(document, "cookie", { set: (value) => cookies.push(value) });
  const context = vm.createContext({
    document,
    localStorage: {
      getItem: (key) => values.get(key) ?? null,
      setItem: (key, value) => {
        if (failWrite) throw new Error("quota");
        values.set(key, value);
      },
      removeItem: (key) => values.delete(key),
    },
  });
  vm.runInContext(script, context);
  return { values, cookies, migrate: context.yunkunBrandMigration };
}

const normal = fixture();
const before = new Map(normal.values);
normal.migrate();
assert.deepEqual(normal.values, before);
assert.equal(normal.cookies.length, 0);
normal.migrate({ apply: true });
assert.equal(normal.values.get("yunkun_task_list_sort"), '{"field":"created_at"}');
assert.equal(normal.values.get("yunkun_task_list_filters"), "current");
assert.equal(normal.values.has("artex_task_list_sort"), false);
assert.equal(normal.values.has("artex_token"), false);
assert.equal(normal.values.has("yunkun_token"), false);
assert.equal(normal.cookies.length, 2);
normal.migrate({ apply: true });
assert.equal(normal.values.get("yunkun_task_list_filters"), "current");

const denied = fixture(true);
const results = denied.migrate({ apply: true });
assert.equal(denied.values.get("artex_task_list_sort"), '{"field":"created_at"}');
assert.equal(denied.values.has("yunkun_task_list_sort"), false);
assert.ok(results.some((result) => result.status === "failed-source-preserved"));
console.log("browser preference migration checks passed");
