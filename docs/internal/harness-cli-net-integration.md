# 石墨办公 × shimo-net CLI：登录态统一与接入

目标：同一台机器、同一系统用户下，石墨办公和任何 Agent 里的 `shimo-net` CLI 共用一份 net 登录态。任一端登录、切号、退出或续期，其他端下一次调用即生效，不需要各自重登。

CLI 0.9.28 起有共享会话，0.9.29 补齐退出可靠性、并发优先级与 Cookie 属性传递；石墨办公侧只需接四个动作。

## 1. 做法

* 真相源是一份会话文件，不是某个进程，也不是某一端当主。
* 两端平级，都可以写；并发写用 `revision` 挡住，不会互相覆盖。
* 有共享记录时优先于钥匙串；没有记录时行为和旧版完全一致，老用户不受影响。
* 双方的浏览器 cookie 库、钥匙串都不动，只把"当前使用中的会话"对齐。

## 2. 两边现在把登录态存在哪

| | shimo-net CLI | 石墨办公 |
|:----|:----|:----|
|主存储|macOS 钥匙串；Windows 为本地文件|Electron userData 下的 `Cookies`（Chromium SQLite）|
|元数据|`~/.shimo-net-cli/credentials.json`（0600）|登录/退出标记文件|
|共享会话|`~/.shimo-net-cli/sessions/<sha256(origin)>.json`（0600）|无|
|跨端可见|否（各存各的）|否|

CLI 侧的细节：

* 钥匙串条目 `service = shimo-net-cli:<sha256(canonicalOrigin)>`、`account = shimo-net`，值是拼好的整条 cookie 头，**没有 Domain / Path / Expires**。
* `credentials.json` 只存 `origin`、`savedAt`、`storage`、`keychainService`，不含 cookie 值。
* 钥匙串不可写时，cookie 退化为写在 `credentials.json` 的 `cookie` 字段（同样 0600）。
* Windows 没有钥匙串，`%AppData%\shimo-net-cli` 下直接落文件。

石墨办公侧以本次核对的内部构建为准：`~/Library/Application Support/com.shimo.desktop.main.test/`。`Cookies` 是 Chromium SQLite，该构建里 `sid` 为明文、文件 0600；登录与退出另有标记文件；网页会话走 Electron 默认 Session，和智能体界面的 Session 分开。正式版目录名不同，结构一致；正式版没有核对，接入前需自行确认这一点。

### 2.1 能不能直接共用

**值可以，文件不行。**

* 值可以：把石墨办公的 cookie 头仅在内存里交给 CLI 执行 `whoami`，返回账号与 CLI 钥匙串账号一致，退出码 0。同一份会话对服务端有效。
* 文件不行，三个原因：
  1. CLI 存的 cookie 没有 Domain / Path / Expires，写回浏览器库会丢属性；
  2. 两边 key 算法不同：CLI 用带端口的 `canonicalOrigin`，石墨办公的退出标记用 `new URL().origin`；
  3. CLI 旧存储没有持久"已退出"状态，复制文件无法表达退出。

所以方案是**另存一份带元数据的会话文件**，两边都读写它，原生存储各自保留。

## 3. 共享会话

### 3.1 位置与格式

```plain
<状态目录>/sessions/<sha256(canonicalOrigin)>.json     权限 0600，目录 0700
默认状态目录 ~/.shimo-net-cli
SHIMO_NET_STATE_DIR 改整体位置，SHIMO_NET_SESSION_DIR 只改 sessions 位置
```

`canonicalOrigin` 是小写 `scheme://host:port`，端口取有效端口（https 缺省 443，http 缺省 80）。`https://shimo.net` 与 `https://shimo.net:443` 命中同一个文件。

```json
{
  "schemaVersion": 1,
  "origin": "https://shimo.net",
  "authState": "signedIn",
  "account": { "id": "94148834", "name": "张三", "email": "zhangsan@example.com" },
  "cookies": [
    { "name": "sid", "value": "...", "domain": "shimo.net", "path": "/",
      "secure": true, "httpOnly": true, "sameSite": "Lax", "expires": 0 }
  ],
  "revision": 4,
  "updatedAt": "2026-11-24T14:06:09Z",
  "source": "login"
}
```

|字段|说明|
|:----|:----|
|`authState`|`signedIn` / `signedOut`；退出时 `cookies` 为空、账号字段清零|
|`cookies`|请求头需要的字段加元数据；`expires` 为 Unix 秒，`0` 表示会话 cookie|
|`revision`|每次写入加 1；宿主写入时用它做乐观锁|
|`source`|`login` / `harvest` / `import` / `renew` / `logout`，仅用于排查|

读这个文件时：不缓存、不写回（写回一律走 CLI 命令）、不要在日志或 UI 里展示 `value`。

`cookies` 的属性取决于导入方给了什么。用纯 cookie 头导入时只有名称和值，`path` 补 `/`，其余属性为空；用 `{"cookies":[...]}` 导入时原样保留。所以这份文件适合 CLI 发请求，不适合直接当浏览器 Cookie 库的写入源。

### 3.2 四条同步路径

|事件|谁写|其他端如何生效|
|:----|:----|:----|
|登录成功|石墨办公或 CLI 调 `session import`|下一条 CLI 命令直接使用；石墨办公读文件或收 watch 事件|
|切号|同上，覆盖写入|`revision` 变化即视为换号，在途任务失效|
|退出|`session logout`（只标记）或 `logout`（同时清本机凭据）|其他 CLI 立刻报"共享会话已退出登录"，不回退旧凭据|
|续期|CLI 收到 `Set-Cookie` 自动合并（`source: renew`）；石墨办公拿到新 cookie 时主动 `import`|双方各自下一次调用使用新值|

一条优先级规则：**退出优先于并发写入。** 记录为 `signedOut` 时，不带 `--expected-revision` 的 `session import` 会被拒绝；只有刚读过当前 revision 的调用方（即确认这是一次新的登录）才能覆盖。这样旧会话不会把已完成的退出改回 `signedIn`。

三点约定：

* **退出是标记，不是删除。** 记录保留 `signedOut`，任何一端都不敢拿旧 cookie 继续用。
* **没有主从。** 最后一次写入生效，`revision` 只用来发现冲突，不用来选举。
* **写前先校验。** `session import` 默认请求 `/users/me`，通过才落盘；不要上报半登录状态。

### 3.3 并发

写入串行：文件锁（`O_EXCL`，15 秒视为残留）+ 临时文件原子替换。带 `--expected-revision N` 时，当前 revision 不匹配返回退出码 1，让调用方重新读取后再提交。

## 4. 石墨办公接入 CLI

|时机|调用|说明|
|:----|:----|:----|
|登录、切号、续期|`shimo-net session import --json`，会话写 stdin|默认校验 `/users/me`；带 `--expected-revision N` 才允许覆盖已退出的记录，也用于并发控制；`--no-verify` 只用于离线排障|
|启动、窗口恢复|读会话文件，或 `shimo-net session status --json`|文件给全量 cookie；`status` 只给元数据|
|用户退出|`shimo-net session logout --json`|只标记共享退出；`shimo-net logout` 会连带清本机钥匙串|
|常驻监听|`shimo-net session watch --interval 1s`|revision 变化时输出一行 JSON，进程不退出|

`session import` 的 stdin 三种写法，任选一种：

```plain
sid=...; fp=...; deviceId=...

{"cookie":"sid=...; fp=...","account":{"id":"...","name":"...","email":"..."}}

{"cookies":[{"name":"sid","value":"...","domain":"shimo.net","path":"/",
             "secure":true,"httpOnly":true,"sameSite":"Lax","expires":0}],
 "account":{"id":"...","name":"...","email":"..."}}
```

前两种只有名称和值；第三种保留属性，适合宿主直接把自己的 Cookie 对象交过来。三种都必须含 `sid`，否则拒绝写入。

宿主侧还要做四件事：

1. **上报**：`CloudSession` 在登录、切号、续期后调用 `session import`；退出调 `session logout`。
2. **拉取**：启动时先读会话文件。和本地 `Cookies` 库冲突时以共享会话为准，不要用旧库反向覆盖。
3. **落地**：收到 watch 事件后更新 Electron session 的 cookie、账号缓存和退出标记，再刷新云页面；切号与退出要让在途任务失效。
4. **工具层**：`spawn` 调二进制（`shell: false`）、参数白名单、固定路径；每次调用前取一次当前会话，不要缓存整段进程环境；未登录直接拒绝，不要传空 cookie。

### 4.1 Cookie 回填规则

共享文件里的 cookie 只保证"能发请求"，不保证属性和浏览器一致。宿主更新 Electron Session 时：

* 以自己 `CloudSession` 里的 Cookie 对象为准，Domain / Path / Secure / HttpOnly / SameSite / Expires 全部沿用原对象，不要从共享文件反推。
* 共享文件只用来核对两件事：当前是否 `signedIn`，以及 `sid` 的值是否已更新。
* 如果宿主手里只有 cookie 头（例如从其他渠道拿到），用第 4 节的 `{"cookies":[...]}` 形式导入并补齐属性；拿不到的属性（Host-only、优先级）无法还原，需要在接入侧明确接受这个近似。

调用时建议注入的环境变量，避免 CLI 自己去找浏览器或旧凭据：

```plain
SHIMO_NET_ORIGIN=<当前云服务 origin>
SHIMO_NET_NO_BROWSER=1
SHIMO_NET_DISABLE_HARVEST=1
SHIMO_NET_STATE_DIR=<与共享会话一致的目录>
```

## 5. CLI 能做什么

`shimo-net` 是 shimo.net 的命令行客户端，覆盖文件管理、内容读取、在线编辑和页面操作。当前版本 0.9.29：**32 条命令、28 个场景、51 项能力**。

### 5.1 命令分类

|类别|数量|命令|
|:----|:----|:----|
|登录与会话|7|`init` `config` `login` `logout` `whoami` `doctor` `session`|
|查找|5|`search` `spaces` `ls` `info` `subscriptions`|
|传输与读取|5|`upload` `import` `pull` `get` `read`|
|组织|7|`new` `mkdir` `rename` `delete` `trash` `restore` `fav`|
|权限|3|`audit`；`share`、`purge` 按设计拒绝，必须页面人工操作|
|打开|1|`open`（独立配置，不注入会话）|
|编辑|3|`editor` `phase2` `phase3`|
|元信息|1|`version`|

### 5.2 编辑能力规模

|入口|动作数|说明|
|:----|:----|:----|
|`editor`|13|7 个可直接走 API，6 个依赖浏览器上下文；含电子表格读写、格式、合并、评论与提及|
|`phase3`|62|45 个编辑动作（文稿 9、幻灯片 15、应用表格 17、表单 4）+ 7 个只读检查 + 3 个页面展示 + 1 个导出 + 6 个电子表格补强|

动作名存在不等于任意场景都可用：`phase3` 的写操作多数需要 CLI 自己拉起浏览器，且要带 `--read-range` 做读回校验。逐项状态见[验收记录](../phase3-acceptance.md)。

### 5.3 使用方式

统一形式：

```plain
shimo-net <command> [参数] [--json] [--no-browser]
```

* `--json`：机器可读输出，成功走 stdout，失败走 stderr（`{"ok":false,"error":"..."}`）。
* `--no-browser`：禁止 CLI 启动浏览器，只对 API 路径有效。
* 链接参数可以直接给 URL，也可以用 `@别名`（先 `fav add`）。

常用示例：

```plain
shimo-net whoami --json
shimo-net search 季度报告 --limit 10 --json
shimo-net ls <文件夹链接> --limit 20 --json
shimo-net read <文档链接> --max-chars 8000 --json
shimo-net new 周报 --type doc --json
shimo-net import ./report.docx --to <文件夹链接> --json
shimo-net get <文件链接> --out ./download --json
shimo-net editor <表格链接> --action sheet-update --payload '{"range":"工作表1!A1:B2","resource":{"values":[["产品","价格"],["A",10]]}}' --json
shimo-net phase3 <文稿链接> --action document-layout-inspect --json
```

命令与动作的完整目录用 `shimo-net schema --json` 读取，参数细节以[命令参考](../cli-reference.md)和运行版本为准。

## 6. 失败与退出码

|情况|表现|
|:----|:----|
|共享会话已退出|退出码 1，`共享会话已退出登录（可能是其他客户端退出）；请重新运行 shimo-net login`|
|`--expected-revision` 不匹配|退出码 1，`共享会话版本已变化；请重新读取后再提交`|
|导入的会话校验失败|退出码 1，`导入的会话不可用：...`，原记录不变|
|退出后不带 revision 导入|退出码 1，`共享会话已退出（revision N），拒绝用旧会话覆盖；确认是新登录请重新读取后带 --expected-revision N 提交`|
|`logout` 时共享写入失败|退出码 1，`本机凭据已清除，但共享退出未写入（其他 Agent 可能仍在使用旧会话）：...`|
|缺 `sid`|退出码 1，`会话缺少 sid，拒绝写入`|
|参数不支持|退出码 2，用法错误|

写操作失败后先读回确认，不要盲目重放。

## 7. 性能与部署形态

`session watch` 是轮询，但不读文件：每轮只 `stat` 会话文件，`mtime` 和大小都没变就跳过。

|指标|实测|
|:----|:----|
|单轮检测（stat 路径）|3.4 µs|
|整条读取 + 反序列化（改前）|23.0 µs|
|1 秒间隔的日均 CPU|约 0.29 秒，占比 0.0003%|
|watch 进程峰值内存|11.6 MB|

结论：性能可以忽略，代价是**延迟上限等于 `--interval`**（默认 1 秒）。给研发的两条建议：

* 一个宿主进程只起一个 watch，不要每个会话或每个工具各起一个。
* 需要更快就把 `--interval` 调到 200–500ms；仍不够再考虑下面的 socket 方案。

后续可选：常驻 socket 服务，用 NDJSON 走 Unix socket（Windows 命名管道），把 `status` / `import` / `logout` 也变成 RPC 并主动推送。前置条件是文件仍是唯一真相源、socket 不可用时自动回退到轮询、懒启动加锁防重复、目录 0700 与 socket 0600。工作量约 350–500 行，Windows 需另写一层。当前没有明确毫秒级需求，不建议现在做。

## 8. 信任边界与生效前提

会话文件 `0600` 防的是同机其他系统用户，不防同一用户下的其他进程。同一用户安装的任何 Agent 都能读到文件里的 cookie，这是共享方案的预期范围，不是漏洞。要按 Agent 隔离就只能换系统用户或容器，那时也不再叫"统一登录态"。

共享成立的前提，缺一条就会退化成"看起来同步了"：

1. 所有 Agent 与石墨办公用同一个状态目录：同一个系统用户，不覆盖 `SHIMO_NET_STATE_DIR` / `SHIMO_NET_SESSION_DIR`。
2. 不要常驻注入 `SHIMO_NET_COOKIE`。它优先于共享会话，退出标记对它无效，只能用于临时排障。
3. CLI 版本需支持共享会话（0.9.28 及以上）；更早的版本只读自己的钥匙串，不参与同步。
4. 石墨办公关闭不影响共享：会话文件独立存在，CLI 照常可用。
5. 退出必须由一端显式调用 `logout` 或 `session logout`；不调用就不会产生 `signedOut` 记录。

## 9. 迁移与兼容

* 不写共享会话的旧版 CLI 完全不受影响：没有记录时按原逻辑读钥匙串/文件。
* 首次接入时如果钥匙串账号和宿主账号不同，以宿主上报为准（`session import` 覆盖）；要保留旧账号先导出或重新登录。
* 共享会话只存请求所需 cookie，不存 Local Storage、Session Storage，这些仍然只有宿主有。
* 不同 origin 各自一份文件，切换环境不会互相清空。
* 建议宿主启动时用 `shimo-net version --json` 和 `shimo-net schema --json` 校验版本；`session` 子命令不存在的旧版本不参与共享。
