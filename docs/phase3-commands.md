# 三期命令参考

**Agent 先区分预览与操作。** 文件链接先内置展示；页面操作优先用具备操作能力的内置工具，否则用可用的外部 Computer Use，尽量复用同一标签页。不因本页有固定动作就先运行它。这里的页面命令会自行启动、关闭或重开本机 Chrome，不会连接 WorkBuddy 等宿主的内置标签页；仅在已选定并获准使用外部 CLI 方案时使用。纯 API/导出动作可配合 `--no-browser`；它阻止本机浏览器，不会转交页面命令。详见[浏览器规则](scenarios.md#浏览器优先级)。

本页描述当前源码中已接入的固定动作与参数，不代表全部线上验收通过。完整场景状态见 [验收记录](phase3-acceptance.md)，原始需求与测试断言见 [用例](phase3-cases.json)。未实现的能力不得由其他动作或本地副本替代。

```bash
shimo-net phase3 <文件链接> --action <动作> --payload '<JSON>' --out ./evidence --json
```

只读检查通常不需要 `--payload`。`--out` 是证据父目录，每次运行生成独立子目录；文稿、幻灯片原地修改仍需已登录的 Chrome。API 优先，浏览器只执行固定的编辑与验证流程，不接受任意 JavaScript。目标链接必须与当前配置的 origin 一致。

## 文稿

字符、文内表格、图形及本人提及参数见 [文稿命令](phase3-document-commands.md)。布局相关动作如下：

| 动作 | payload 示例 | 范围 |
|---|---|---|
| `document-layout-inspect` | 无 | 只读导出段落、节及主题结构，取得真实段落号与原文。 |
| `document-paragraph` | `{"paragraph":2,"text":"精确原段落","attrs":{"firstLineChars":2,"lineMultiple":1.5,"shading":"#EEEEEE"}}` | 段落号从 1 开始；缩进单位为字符，行距为倍数。也支持 `heading:1` 至 `6`、`numbering:"decimal"`。 |
| `document-page` | `{"marginsCm":{"top":2,"bottom":2,"left":1.5,"right":1.5},"columns":2,"background":"#EAF3FF"}` | 当前仅支持单节文稿；边距必须同时给出四边，单位厘米；分栏 1–4。各组设置可独立使用。 |
| `document-theme` | `{"themeFonts":{"major":{"latin":"Arial","ea":"宋体","cs":"Arial"},"minor":{"latin":"Arial","ea":"宋体","cs":"Arial"}},"resetSupplemental":true}` | 必须完整提供两组字体，并明确允许原生主题重建补充语言映射及 Normal/标题主题继承；显式文字字体不应被误改。 |

`document-replace` 的参数为 `{"find":"旧版测试","replacement":"新版测试","expectedCount":3}`，按精确字面文本及预期次数替换。`document-goto` 使用 `{"page":2}` 验证从 1 开始的物理页面位置，不持久化光标。两者均保留导出与页面证据，不能仅凭工具栏或返回值认定完成。

## 幻灯片

页面、文本、占位符、形状、表格、动画、放映设置和 PDF 导出的参数见 [幻灯片命令](phase3-slide-commands.md)。先运行 `slide-design-inspect` 获取真实页面和对象标识。

`slide-text-format` 的 `color` 是文字填充；文字轮廓使用独立 `outlineColor/outlineWidth`（磅，默认 1），核对真实文本 run 的线条定义。`slide-shape-format` 的线条不能代替文字轮廓。`slide-layout` 只切换当前母版内的版式；整套主题使用独立 `slide-theme` 动作。本轮已验证官方“雪”主题及随后第二页版式变更；原主题写命令的验证失败与独立只读恢复分开记录，不代表其他官方主题均已通过。具体范围见 [33.1 验收记录](phase3-acceptance.md)。

## 应用表格

先用 `table-inspect` 获取真实数据表、字段、记录和视图 GUID。所有字段键使用 GUID，单选值使用选项 GUID；不得把显示名称当成标识。

| 动作 | payload 形状 | 约束 |
|---|---|---|
| `table-create` | `{"name":"测试表","desc":"合成测试数据"}` | 新建主文本字段、空记录及表格视图。 |
| `table-update` | `{"tableGuid":"实际GUID","name":"新名称","desc":"新说明"}` | 名称和说明可分别修改。 |
| `table-duplicate` | `{"tableGuid":"实际GUID","name":"测试副本"}` | 只接受当前可验证的简单字段，保留记录值；拒绝同步、表单关联及不支持的复杂结构。 |
| `table-delete` | `{"tableGuid":"实际GUID"}` | 拒绝最后一个数据表、被引用目标及同步/表单关联表；其他表必须保持不变。 |
| `table-field-add` | `{"tableGuid":"实际GUID","fields":[{"name":"优先级","type":"SingleChoice","options":["高","低"]}]}` | 类型支持 `SingleLineText/Number/SingleChoice/Date`。 |
| `table-field-update` | `{"tableGuid":"实际GUID","fieldGuid":"实际字段GUID","name":"新字段名"}` | 当前只改名。 |
| `table-field-convert` | `{"tableGuid":"实际GUID","fieldGuid":"实际字段GUID","type":"Number"}` | 仅文本与数值无损回转；拒绝会丢失格式的 `001` 等值及依赖字段。 |
| `table-field-delete` | `{"tableGuid":"实际GUID","fieldGuids":["实际字段GUID"]}` | 拒绝主字段及任何已识别依赖。 |
| `table-record-add` | `{"tableGuid":"实际GUID","records":[{"实际字段GUID":"合成测试值"}]}` | 支持批量新增；按新增数与逐字段值核验。 |
| `table-record-update` | `{"tableGuid":"实际GUID","where":{"条件字段GUID":"旧值"},"values":{"目标字段GUID":"新值"}}` | 非空 AND 等值条件；拒绝隐式全表修改和零命中。 |
| `table-reference-add` | `{"tableGuid":"实际GUID","referenceTableGuid":"另一实际GUID","name":"关联测试"}` | 同一应用表格内建立真实双向字段引用，核对两侧身份及既有数据。 |
| `table-reference-set` | `{"tableGuid":"实际GUID","fieldGuid":"关联字段GUID","recordGuid":"源记录GUID","referenceRecordGuids":["目标记录GUID"]}` | 使用已建立的双向字段设置真实记录关联；空数组表示清空此记录关联。拒绝已有不对称、孤立、跨库或视图限定引用，不把字符串冒充关联对象。 |

日期字段接受带时区的 RFC3339 字符串，例如 `2026-11-20T09:00:00+08:00`；CLI 转换为服务使用的日期序号，不接受含义不清的数值时间戳。

| 视图与图表动作 | payload 形状 | 约束 |
|---|---|---|
| `table-view-create` | `{"tableGuid":"实际GUID","viewGuid":"已有视图GUID","name":"测试看板","type":"Kanban","groupFieldGuid":"单选字段GUID"}` | `viewGuid` 为锚点视图。`Grid` 无额外字段；`Calendar` 需要日期类型的 `startFieldGuid/endFieldGuid`。 |
| `table-view-update` | `{"tableGuid":"实际GUID","viewGuid":"实际视图GUID","name":"新视图名"}` | 还可设置明确的 `filters/groups/sort`；只接受当前可验证结构。 |
| `table-view-delete` | `{"tableGuid":"实际GUID","viewGuid":"实际视图GUID"}` | 拒绝默认、最后共享、个人/锁定/已共享视图及跨表、仪表盘依赖。 |
| `table-chart-create` | `{"tableGuid":"实际GUID","fieldGuid":"分组字段GUID","name":"测试计数","type":"Column"}` | 支持 `Column/Line/Bar/Pie` 计数图；拒绝空数据，核对图表身份、来源及真实渲染聚合值。创建默认图表不等于已验收调整样式。 |
| `table-chart-style` | `{"dashboardGuid":"实际仪表盘GUID","componentGuid":"实际组件GUID","themeColor":1}` | 只修改该组件的原生配色方案，范围 0–10；拒绝无变化，核对配置和前后真实渲染颜色。不是任意 RGB 系列颜色接口。 |

`table-view-update` 的筛选、分组及排序配置示例：

```json
{"tableGuid":"实际GUID","viewGuid":"实际视图GUID","filters":{"conjunction":"And","conditions":[{"fieldGuid":"数值字段GUID","operator":"GreaterThanEqual","value":10}]},"groups":[{"fieldGuid":"分组字段GUID","order":"Asc"}],"sort":{"rules":[{"fieldGuid":"数值字段GUID","order":"Desc"}],"keepSorted":true}}
```

筛选支持 `And/Or` 及 `Equal/NotEqual/GreaterThan/GreaterThanEqual/LessThan/LessThanEqual`；大小比较只接受数值字段，`filters:null` 清空筛选。分组和排序至多三项，方向为 `Asc/Desc`；当前精确排序只支持数值字段，非空值必须为有限数值，空值在升序和降序时均排到末尾。

视图创建保留 `viewGuid` 的插入锚点：新视图插在其后，后续既有视图顺序按原生规则后移；CLI 从写前快照预测完整顺序。原生创建协议不携带排序，CLI 在同一 compose 中显式追加空排序规则，随后精确核验，部分执行仍失败且不重放。

日历展示证明使用实际原生视图和独立 API 日期值，记录原始 `documentTimezone`、`browserTimezone`、`effectiveCalendarTimezone` 及 `calendarTimezoneSource`。当前原生编辑器对其内置列表使用固定时差（包括夏令时地区），列表之外的标识会回退浏览器本地时区；例如本轮原始 `UTC` 实际回退 `Asia/Shanghai`，证据会明确标注原因，不能将其描述为“按 UTC 展示”。新配置浏览器重新读取绑定视图的时区并核对日期。日历验收将隔离浏览器可视区域设为 1440×1400，以展开月视图中可能折叠为“+N 个记录”的事件；仍缺少任何预期记录时拒绝成功，不修改文档时区或事件内容。

关联字段新增还会在同一次 CLI 前置检查中打开目标表的原生字段类型菜单，读取实际绑定表的可用类型和 `FieldOptions`，随后取消未保存对话框。只有菜单明确提供可用 `ReferenceRecord` 时才发送 API 写请求；菜单未初始化、绑定不符或该类型不可用均拒绝写入，并保存 `reference-capability.json`（若已读到）及 `mutationAttempted:false`。菜单缺项只说明当前表当前页面不提供该能力，不凭一次 HTTP 400 推断整个部署禁用了关联功能，也不修改隐藏功能配置。

## 表单

完整题型、批量编辑、分支、发布和提交参数见 [表单命令](phase3-form-commands.md)。草稿、预览、发布、填写、结果统计是不同阶段；空答卷或初始化草稿不能算完整通过。

## 失败处理

在 macOS ARM64 上，若检测到 Chrome 可执行文件包含 ARM64 架构，三期隔离浏览器通过系统 `arch -arm64` 启动，避免该测试会话意外使用 Rosetta；其他平台或仅有 Intel 架构的浏览器保留原启动方式。不修改日常浏览器配置。浏览器启动后会在导航前将测试页置于前台。

编辑器启动只尝试一次，不因传输错误关闭并创建新窗口。初始就绪在同一页面使用最多 300 秒的总预算做只读等待；慢加载不会在 120 秒时自动刷新，导航期间短暂缺少执行上下文也不会因固定次数限制而提前重开。控制连接中断时，最多尝试重新连接同一个 CDP 目标一次，不刷新页面、不重启浏览器，恢复仍消耗原预算；恢复记录保存在 `startup-recovery.json`。明确登录、权限或 JavaScript 动作错误直接失败；超时保存诊断并报错，不重跑写入。该等待策略的自动化测试不代表当前部署已实测通过。

文稿的字符、图形、本人提及和文内表格动作还会在方法可用后，最多等待 30 秒确认编辑角色和可编辑状态。该阶段只读取状态，避免编辑器方法先出现、权限状态稍后才到达时立即误报；超时仍然失败，不重放写入。

未知或断开的在线协作名单、其他用户在线、权限不明都会阻止写入。用户确认本轮无第二测试账号，真实双账号冲突验收仍保留待验收。

`phase3 --relogin` 不会自动重放写命令。若提示“可能已生效”，先查看证据、读回原文件；不要因为客户端错误重复新增、删除、发布或提交答卷。表单批次部分完成时不能重放整个批次。返回 `verified:true` 也只证明结果中明确列出的验证范围，不等于整项需求通过。

### 批量记录失败后的逐条报告

`table-record-add` 和 `table-record-update` 一旦尝试提交后失败，会先保留原始 `failure.json`，随后在 30 秒总预算内最多发送 3 个只读 GET（revision、完整记录表、revision），保存 `record-outcomes.json`。报告按原请求计划的记录 ID 核对完整 `valueMap`；新增项 `inputIndex` 对应输入顺序，更新项对应匹配 ID 排序。

| status | 判定 | source |
| --- | --- | --- |
| `applied` | 稳定且已推进的 revision 中，该 ID 的完整值等于请求后预期 | `stable_readback` |
| `not_applied` | 同样的完整快照中，预定新增 ID 不存在，或待更新 ID 的完整值仍等于写前值 | `stable_readback` |
| `unknown` | 值混合、待更新记录缺失，或读取失败、401、revision 未推进/读取期间变化、快照缺失/重复 ID | 有可靠快照但结果不符时为 `stable_readback`，无法取得可靠快照时为 `unknown` |

逐条状态描述一次观察，不推断服务端批次原子性；`not_applied` **不代表可以安全重试**，请求可能仍在处理。读回不刷新认证、不重新提交写请求，`--relogin` 也不会重放整个命令。本版本没有跨进程手动重试的幂等接口。

原始写入/验收错误保留在 `originalError`，报告始终 `ok:false`、`verified:false`、`mayHaveApplied:true`、`automaticReplayAllowed:false`。即使所有目标记录均为 `applied`，其他字段、视图或非目标记录损坏仍不能被逐条报告转为成功；完整状态验收错误单独记录。报告无法写入时，命令继续返回失败并明确说明证据保存错误。

诊断只读取完整记录表，不额外展开延迟加载的视图。`wholeTableError` 表示这次诊断不能证明完整状态符合请求；它也可能来自未展开的视图元数据，不能单凭该字段断言发生了非目标数据损坏。

真实线上新增和条件更新的成功验收，与离线生产命令路径的部分保存、401、读回失败等故障注入分开记录。后者验证失败保护，不是线上出现部分提交的证明。
