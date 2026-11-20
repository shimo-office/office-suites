---
name: shimo-net-cli
display_name: 石墨办公 CLI
display_name_en: shimo-net CLI
description_zh: 用 shimo-net 直连 shimo.net，搜索、读取、上传、下载、新建、改名和处理表格；文件链接优先内置展示，页面编辑按实际浏览器工具能力执行，权限变更和彻底删除由人工完成。
description_en: Use shimo-net with shimo.net to search, read, upload, download, create, rename, and edit sheets. Show file links in the host preview and use available interactive browser tools for page actions; handle permission changes and permanent deletion manually.
description: 用 shimo-net 直连 shimo.net 完成文件和表格操作；文件链接优先内置展示，需要页面交互时选用可操作的浏览器工具，高风险权限和彻底删除交给人工。
whenToUse: 用户要用石墨 shimo.net 找文件/空间、上传或导入、拉取下载、权限巡检、新建/改名、读文档、记常用文件夹时；高风险权限和不可逆操作必须人工处理。
examples_zh:
  - "帮我搜索石墨里名为‘季度方案’的文件，并返回链接。"
  - "在这个表格的工作表1 A1:C3 写入课表内容，然后打开页面核对结果。"
  - "列出我订阅的文件，选择一个打开查看最近更新。"
examples_en:
  - "Search Shimo for the file named 'Quarterly Plan' and return its link."
  - "Write the timetable to Sheet1 A1:C3 and open the page to verify it."
  - "List my subscribed files and open one to review its latest update."
version: 0.9.19
author: shimo-office
---

# shimo-net CLI

涉及 shimo.net 或客户私有部署的文件与办公操作时使用。需要 CLI 时，调用统一为 `shimo-net`，按安装渠道完成入口配置，不猜测仓库脚本路径。先按下节区分链接展示与实际操作。

## 先选执行通道，再检查 CLI

**先用内置预览展示原文件链接，再按实际工具选择操作通道。内置浏览器只能预览也可以；没有操作能力时使用可控制的外部浏览器。** 完整规则以[浏览器优先级](https://github.com/shimo-office/office-suites/blob/main/docs/scenarios.md#浏览器优先级)为准。

1. 用户给出链接或页面操作开始前，优先在内置浏览器或预览面板展示原文件。新建、导入成功后展示服务返回的真实链接；编辑完成并核验后复用预览展示同一文件。无法展示时返回可点击链接，不循环打开，不为了预览先导出 PDF 或登录 CLI。预览打开成功不等于已登录、编辑器就绪或保存成功。
2. 需要页面操作时，先检查当前会话工具。内置浏览器具备所需的读取、点击、截图能力就用它；只能预览时，使用实际可用的 Computer Use / Browser Use 操作外部浏览器，优先复用会话和标签页。没有可操作工具就说明限制，不猜工具名称或强制安装 `agent-browser`。沿用用户已有授权，不重复询问。
3. 加载慢时在原页做有总时限、按间隔的只读等待，不反复刷新、关闭、重开或更换浏览器配置。超时报告阶段和错误，不重跑整项任务；写入可能成功时先读回，禁止盲目重放。工具自行回收会话时检查实际设置，不猜保活参数或循环拉起。不得索取、手动注入或跨浏览器搬运 cookie，也不盲目清空用户配置。

`phase3` 页面动作、`editor --verify`、`open`、`login` 和评论/提及读取可能自行启动本机浏览器，**不会接管宿主预览或其他工具的操作会话**。CLI 的等待策略不能控制 WorkBuddy 或 `agent-browser` 的窗口生命周期。纯 API 查询和写入仍使用 CLI；纯页面任务不以 CLI 初始化为前提。

## 需要 CLI 时的检查

- 读取 `shimo-net schema --json` 确认动作与参数。尚未选择 CLI 浏览器方案时，每次调用加 `--no-browser`，包括 `doctor`、`whoami` 和 API 命令；也可在每个任务 shell 设置 `SHIMO_NET_NO_BROWSER=1`，不修改全局配置。已选择外部 Computer Use 不代表需要启动 CLI 的独立浏览器。
- `--no-browser` 阻止 CLI 启动浏览器及会话收取，**不会把命令转交其他浏览器**。页面动作因此拒绝时回到已选操作工具，不直接移除开关重试；截图使用实际操作通道的工具，不能要求只有预览能力的面板截图。
- CLI 缺失时按[安装说明](https://github.com/shimo-office/office-suites/blob/main/docs/install.md)处理；先 `session status --no-browser --json`，需要核对 API 账号时用 `whoami --no-browser --json`。`doctor --no-browser --json` 仅诊断 CLI 环境，不能据此否定预览或其他浏览器工具可用。
- CLI 凭据与各浏览器登录态独立。未登录时说明 API 通道状态；确需 CLI 登录或会话收取时说明其外部浏览器要求，并按既有授权处理。401/域名不匹配不能静默切换账号，切账号用 `login --new`，完成后核对 `whoami`。

## 按执行方式选入口

| 类型 | 入口与边界 |
|---|---|
| API | 文件命令、`editor` 的表格 API；先看 schema 与[命令参考](https://github.com/shimo-office/office-suites/blob/main/docs/cli-reference.md)。表格写入、追加、插删行、合并与底色使用相应 API，底色为 `sheet-format` 的 `attrs.background`。 |
| 页面操作 | 使用具备操作能力的内置或外部浏览器；评论/提及的 CLI 读取同样需要本机浏览器，也可在已选操作页面读取。稳定 API 不提供的冻结、排序、筛选、数据验证等使用页面菜单。外部 CLI 通道的历史/评论 show/hide 仅展示面板并需 `--verify`；save-version/comment-add/print 均拒绝临时面板工作流，须在持久页面完成并核验。 |
| 人工禁用 | 文件锁定、限制编辑、高级权限、成员级授权、彻底删除及权限变更必须由用户在石墨网页操作。CLI 的拒绝不是失败后改用自动化绕过的许可。 |

完整场景、自然语言话术与每项边界见[唯一场景清单](https://github.com/shimo-office/office-suites/blob/main/docs/scenarios.md)，能力计数见[内部矩阵](https://github.com/shimo-office/office-suites/blob/main/docs/internal/capability-matrix.md)。本技能不另维护场景表。

## 写入与验证

- 明确目标 GUID、对象或区域及影响范围，使用用户已授权的目标。`new` 与 `mkdir` 的区别、文件类型和参数以 schema 为准。导入后默认私有；需要调整权限时交用户确认并在网页人工设置。
- API 返回成功只代表受理。写后核对读回值、目标导出结构及真实页面截图；空行插入和合并不能仅靠 `sheet-values` 判断。没有截图时只能报告“已提交，待页面核验”。
- 页面操作使用已选浏览器工具；选择外部 CLI 通道时使用现有固定动作和有类型约束的参数。禁止以本地改文件再导入副本替代原地编辑，也不能用包结构检查冒充动作通过。
- 编辑器慢加载可有限等待和只读重试。操作可能已经生效时先只读核对，不重放新增、删除、发布或答卷提交；所有写命令及 `phase3 --relogin` 不会自动重跑整条命令。失败须保留原错误、阶段和可能已写入的提示。
- 截图必须来自实际页面，排除登录页、加载页和空白页。结果链接应能打开，输出实际截图绝对路径；展示时使用 Markdown 图片。导出物必须存在且可读，不用模拟终端图、示意截图或 `verified:true` 单独宣称验收通过。
- 只读检查与测试通过数不代表全部线上验收完成；高级办公动作与未验收范围见[阶段验收记录](https://github.com/shimo-office/office-suites/blob/main/docs/phase3-acceptance.md)。

## 会话与输出

私有部署使用 `SHIMO_NET_ORIGIN=https://<客户域名>`，也可先 `init --origin`。API 通道使用已存 CLI 凭据，页面通道使用所选浏览器的会话；仅选定并获准使用 CLI 浏览器登录方案时才收取本机浏览器会话。不把 cookie、sid、Authorization 或账号明细写进对话、示例、截图文件名及报告。

默认用 `--json` 解析结果。只报告用户需要的内容、文件链接、可核对的结果与限制；无权限和验证失败如实说明。会话和 401 处理细节见[安装说明](https://github.com/shimo-office/office-suites/blob/main/docs/install.md)。
