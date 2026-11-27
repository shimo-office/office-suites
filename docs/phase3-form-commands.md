# 三期表单命令参考

通用命令：`shimo-net phase3 "$FORM_URL" --action <action> [--payload '<JSON>'] --out ./evidence --json`。`FORM_URL` 必须是有权限的表单链接。`--out` 是证据父目录；每次生成独立 `phase3-*` 子目录。该实现使用已登录账号：API 优先；需要编辑器 SDK、原生预览/填写页面时使用隔离 Chrome。

| action | 最小 payload | 作用与成功范围 |
|---|---|---|
| `form-draft` | 无 | 从签名服务端内容读取并解码完整草稿；不以当前页面内存当已保存正文。 |
| `form-edit` | 见下方 typed operation | 修改题目、顺序、原生条件规则或标题/描述/结果页；每步独立服务端读回，最后新 Chrome profile 完整比对。只证明草稿，不代表已发布。 |
| `form-settings` | `{"deadline":"2026-11-20T18:00:00+08:00","timezone":"Asia/Shanghai"}` | 仅设置未来截止时间；IANA 时区与日期偏移必须相符。可省略 timezone。内部按官方协议转成 UTC 偏移小时数。不得改填写权限。示例日期需换成实际未来时间。 |
| `form-publish` | 无 | 发布或重新发布当前已验证草稿；独立读回 `normal`、完整发布正文及权限/通知字段，再核验新 profile。配置了通知/提醒或测验模式时拒绝。 |
| `form-preview` | 见分支 payload | 在真实未提交预览页选择单选分支并验证题目可见性/顺序，不提交答卷。 |
| `form-respondent` | 与 preview 相同 | 在真实已发布填写页验证分支，同时核对填写页标题/描述/身份；可验证原生可达题序。要求已开放收集。 |
| `form-submit` | `{"answers":{"text01":"合成测试A","pick01":"yes01","more01":"补充说明","rate01":5}}` | 默认单次 API 提交，要求管理权限用于独立结果核验；恰好增加一份答卷、已有答卷不变、新答案精确一致才成功。不是通用匿名填写客户端。 |

`form-submit` 可另加 `--confirm-text '测试已收到'`：在真实填写页填入同一组答案，点击原生“提交”按钮一次；要求应用自行进入 Confirmation 页并实际显示文本，随后仍执行独立结果核验。不会直接改页面状态或伪造结果页。该选项用于验收提交后页面。

提交请求或原生按钮只执行一次。提交后最多读取 15 轮，所有分页请求和轮间等待共享 120 秒预算；单页上限为 90 秒或总预算剩余时间中较短者，轮间间隔 1 秒。预算到期即取消正在进行的请求，不再发送下一页或继续等待。仅对新答卷尚未被索引，或服务端明确返回“本页条数超过索引总数”的暂态重读；通用分页仍报错，不把不一致快照当作完整结果。暂态中已解析的行也会先核验：已见旧答卷改变、未知新 ID、多份新增或跨次新增 ID 改变均立即失败；部分页面暂未包含旧 ID 不等于删除，最终完整读回仍要求旧答卷全部保持且总数精确加一。权限与未知结构等错误立即失败。每次读回记录在 `readback-attempts.json`：`totals` 中的 `-1` 仅代表明确的索引不一致，不能当作实际答卷数。等待不会重试提交或自动重新取登录态；超时仍须独立读回并保留原失败记录。

## form-edit：单动作及批量

每个请求可以是一个 operation；批量形状是 `{"operations":[<operation>,<operation>]}`，支持 1–20 步。批量在第一步写入前验证完整计划；不提供事务回滚。

新建表单第一次打开时，编辑器会将文件名初始化为表单标题，保存可能稍晚于页面显示。CLI 在写前最多读取 6 次服务端草稿，间隔 1 秒，等待它与原浏览器草稿完全一致；不忽略标题差异、不重复写入。等待期间正文、目标或权限变化会立即中止；持续不一致时将两份快照保存在 `preflight-draft-mismatch.json`。

新增文本题：

```json
{"operation":"question-add","question":{"guid":"text01","type":"text","required":true,"subject":"测试答卷标记","question":{}}}
```

新增单选题：

```json
{"operation":"question-add","question":{"guid":"pick01","type":"single_choice","required":true,"subject":"是否需要跟进？","question":{"options":[{"type":"normal","value":"yes01","label":"需要跟进"},{"type":"normal","value":"no001","label":"不需要"}]}}}
```

新增评分题：

```json
{"operation":"question-add","question":{"guid":"rate01","type":"rate","required":true,"subject":"满意度评分","question":{"shape":"star","sum":5}}}
```

支持 `text` / `multiple_text`（默认配置）、`single_choice` / `multiple_choice` / `dropdown_list`（normal 选项）、`rate`（star，sum 为 1–10 整数）。多选仅支持无限制配置，可加 `"limitCount":false`。问题 GUID 和选项 value 为 4–64 位字母/数字/下划线/短横线；必须唯一。已有未支持的题型校验配置会使测试提交拒绝执行，避免直接 API 绕过前端规则。

其他最小动作（GUID 先通过 `form-draft` 获取；索引从 0 开始）：

```json
{"operation":"question-update","guid":"more01","patch":{"required":true,"subject":"补充说明"}}
```

```json
{"operation":"question-reorder","from":2,"to":1}
```

```json
{"operation":"question-delete","guid":"unused01"}
```

```json
{"operation":"meta","patch":{"title":"三期验收表单","desc":"仅合成测试数据"}}
```

```json
{"operation":"meta","patch":{"confirmation":{"type":"custom","confirmation":{"showTitle":true,"content":"<p>测试已收到</p>"}}}}
```

删除被规则/组件引用的题目会失败；应先明确调整引用。更新 question 配置时提供该题型的完整 question 对象。

## 原生条件分支跳过

以下动作要求 `pick01`、其选项 `yes01` 及位于其后的题目 `more01` 已存在：

```json
{"operation":"rule-add","trigger":"pick01","options":["yes01"],"targets":["more01"]}
```

更新规则：`{"operation":"rule-update","guid":"实际规则GUID","trigger":"pick01","options":["yes01"],"targets":["more01"]}`；删除规则：`{"operation":"rule-delete","guid":"实际规则GUID"}`。新规则 GUID 由真实 SDK 生成，读回草稿获取。CLI 支持单个 `select_option` 条件和后续题目的 `show_question` 效果，不接受任意 SDK 调用。

这是平台原生条件规则：未命中的目标题从可达问题集合中移除；命中时按原始题序加入。官方填写页的题目显示、必填校验和提交答案筛选都使用该集合。因此可验证“选择不需要后跳过补充题”，包括跳过隐藏的必填题；**不声称支持任意页面跳转、任意目标页导航或所有复杂逻辑规则**。CLI 没有用 CSS 人为隐藏题目。

完整分支 payload（题目顺序示例为 text01 → pick01 → more01 → rate01）：

```json
{"trigger":"pick01","branches":[{"option":"yes01","visible":["more01"],"hidden":[],"sequence":["text01","pick01","more01","rate01"]},{"option":"no001","visible":[],"hidden":["more01"],"sequence":["text01","pick01","rate01"]}]}
```

`trigger` 必须是单选题。`visible` / `hidden` 均必填且至少有一个断言；`sequence` 可选，用于断言完整可见题序。填写页核对原生答案中的已选选项，并比对 SDK 原生 `getVisibleQuestions()` 题序。预览页面从目标输入框所绑定的 React `HostRoot.current` 当前树读取真实预览 form 的答案，先核对标题和完整题目 GUID 集合，不能使用旧 alternate 节点或只凭题目布局推断选项。两种模式都在最终同一次同步读取中再次核验已选选项、DOM 可见性和题序，才输出 `selectionVerified:true`。平台自定义单选控件的 `input.checked` 可能与界面状态不同，因此仅作为诊断记录，不单独用来判定分支成功。预览签名 query 只用于导航，证据仅保存不含 query 值的路由及键名；保留官方 query 顺序以兼容 `/preview?t=` 路由。

no 分支提交示例（即使 more01 必填也应完全省略它）：

```json
{"answers":{"text01":"合成测试B","pick01":"no001","rate01":3}}
```

答案键是问题 GUID；单选/下拉值是选项 value，多选是 value 数组，文本是字符串，评分是范围内整数。拒绝未知题目、隐藏题答案、缺失的可达必填题和不支持的题型规则。

`text01` 等只是本页示例 ID。提交前用 `form-inspect` 查看**已发布**题目；`form-draft` 中新加的题目需先发布才可提交。若新增题目失败且未写入，后续提交该题 ID 会被正确拒绝，不能用自动新增题目或丢弃未知答案的方式绕过。

## 失败、重放与证据

- 退出码非零、`ok:false` 或验证失败都不能视作成功。`actionVerified:true` 只覆盖结果里的 `scope`。
- `form-edit` 失败时检查 `completedSteps`、`currentStep`、`currentMayHaveApplied`；已完成步骤保持原状，**禁止重放整个批次**。
- 设置/发布/提交只发送一次修改请求；`mayHaveApplied:true` 表示服务端可能已改变，必须先读回实际状态，不能因客户端报错再提交一遍。
- UI 提交后即使未观察到结果页，也可能已新增答卷；先核对提交前后分页结果和测试标记，不能重复点击。
- 只有确认失败发生于写入前，且当前状态仍满足预期，才可修正后重新执行对应动作。正常只读预览可重试；预览选择不等于提交。
- 草稿成功依据为独立服务端持久化读回 + 新 profile；SDK 的 `hasUnsaved()` 实测恒返回 false，不能当作保存证据。
- 发布、预览、填写、提交、CSV/统计是不同验收层。完整场景必须具备对应证据；未发布草稿、初始化 `unpublished` 或空答卷导出不能算整项通过。

内部协议核对：当前官方 `edit.command.publishForm` 对正常发布和初始化均传 `rev:1`；CLI 保留这一协议值并严格核对读回，绝不把它当 OT 草稿 revision。重新发布有效性靠完整 publishedContent、新旧 hash 和真实填写页变化证明。

本参考描述实现接口与安全边界，**不是尚未执行的线上场景通过声明**。实际执行结果见 [验收记录](phase3-acceptance.md)，并以独立 evidenceDir、命令记录及最终报告为准。
