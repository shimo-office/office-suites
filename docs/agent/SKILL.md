---
name: shimo-net-cli
display_name: 石墨办公 CLI
display_name_en: shimo-net CLI
description_zh: 用 shimo-net 直连 shimo.net，搜索、读取、上传、下载、新建、改名和处理表格；页面编辑通过宿主浏览器，权限变更和彻底删除由人工完成。
description_en: Use shimo-net with shimo.net to search, read, upload, download, create, rename, and edit sheets. Use the host browser for page actions; handle permission changes and permanent deletion manually.
description: 用 shimo-net 直连 shimo.net 完成文件和表格操作；需要页面交互时使用宿主浏览器，高风险权限和彻底删除交给人工。
whenToUse: 用户要用石墨 shimo.net 找文件/空间、上传或导入、拉取下载、权限巡检、新建/改名、读文档、记常用文件夹时；高风险权限和不可逆操作必须人工处理。
examples_zh:
  - "帮我搜索石墨里名为‘季度方案’的文件，并返回链接。"
  - "在这个表格的工作表1 A1:C3 写入课表内容，然后打开页面核对结果。"
  - "列出我订阅的文件，选择一个打开查看最近更新。"
examples_en:
  - "Search Shimo for the file named 'Quarterly Plan' and return its link."
  - "Write the timetable to Sheet1 A1:C3 and open the page to verify it."
  - "List my subscribed files and open one to review its latest update."
version: 0.9.17
author: shimo-office
---

# shimo-net CLI

涉及 shimo.net 或客户私有部署的文件与办公操作时使用。仓库内入口为 `./shimo`，安装后为 `shimo-net`。

## 执行前

1. 读取 `shimo-net schema --json`，确认动作与参数；用 `shimo-net doctor --json` 检查环境，再用 `shimo-net whoami --json` 验证会话。CLI 缺失时按[安装说明](https://github.com/shimo-office/office-suites/blob/main/docs/install.md)处理，不用示例输出冒充执行。
2. 先检查 `session status --json`；仅无已存凭据时自动复用浏览器会话。401/域名不匹配时显式 login，不通过 harvest 静默切回其他账号；用户明确选择浏览器账号时才执行 session harvest。切账号必须 `login --new`；`logout` 后普通 `login` 仍可能复用原浏览器账号。
3. 浏览器使用顺序只以[场景文档的浏览器优先级](https://github.com/shimo-office/office-suites/blob/main/docs/scenarios.md#浏览器优先级)为准，按当前宿主实际提供的能力选择；不要根据旧版本断言某个宿主没有内置浏览器。

## 按执行方式选入口

| 类型 | 入口与边界 |
|---|---|
| API | 文件命令、`editor` 的表格 API、只读评论/提及接口；先看 schema 与[命令参考](https://github.com/shimo-office/office-suites/blob/main/docs/cli-reference.md)。表格写入、追加、插删行、合并与底色使用相应 API，底色为 `sheet-format` 的 `attrs.background`。 |
| 页面操作 | 稳定 API 不提供的冻结、排序、筛选、数据验证等使用页面菜单。历史/评论 show/hide 仅展示面板并需 `--verify`；save-version/comment-add/print 均拒绝临时面板工作流，须在持久页面完成并核验。 |
| 人工禁用 | 文件锁定、限制编辑、高级权限、成员级授权、彻底删除及权限变更必须由用户在石墨网页操作。CLI 的拒绝不是失败后改用自动化绕过的许可。 |

完整场景、自然语言话术与每项边界见[唯一场景清单](https://github.com/shimo-office/office-suites/blob/main/docs/scenarios.md)，能力计数见[内部矩阵](https://github.com/shimo-office/office-suites/blob/main/docs/internal/capability-matrix.md)。本技能不另维护场景表。

## 写入与验证

- 明确目标 GUID、对象或区域及影响范围，使用用户已授权的目标。`new` 与 `mkdir` 的区别、文件类型和参数以 schema 为准。导入后默认私有；需要调整权限时交用户确认并在网页人工设置。
- API 返回成功只代表受理。写后核对读回值、目标导出结构及真实页面截图；空行插入和合并不能仅靠 `sheet-values` 判断。没有截图时只能报告“已提交，待页面核验”。
- 使用现有固定动作和有类型约束的参数；禁止以本地改文件再导入副本替代原地编辑，也不能用包结构检查冒充动作通过。
- 编辑器慢加载可有限等待和只读重试。操作可能已经生效时先只读核对，不重放新增、删除、发布或答卷提交；所有写命令及 `phase3 --relogin` 不会自动重跑整条命令。失败须保留原错误、阶段和可能已写入的提示。
- 截图必须来自实际页面，排除登录页、加载页和空白页。结果链接应能打开，输出实际截图绝对路径；展示时使用 Markdown 图片。导出物必须存在且可读，不用模拟终端图、示意截图或 `verified:true` 单独宣称验收通过。
- 只读检查与测试通过数不代表全部线上验收完成；高级办公动作与未验收范围见[阶段验收记录](https://github.com/shimo-office/office-suites/blob/main/docs/phase3-acceptance.md)。

## 会话与输出

私有部署使用 `SHIMO_NET_ORIGIN=https://<客户域名>`，也可先 `init --origin`。优先从本机凭据存储与已登录浏览器复用会话，不把 cookie、sid、Authorization 或账号明细写进对话、示例、截图文件名及报告。

默认用 `--json` 解析结果。只报告用户需要的内容、文件链接、可核对的结果与限制；无权限和验证失败如实说明。会话和 401 处理细节见[安装说明](https://github.com/shimo-office/office-suites/blob/main/docs/install.md)。
