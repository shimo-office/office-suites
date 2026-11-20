# Issue #1：31 项逐项复核与修复

> 公开摘要：本页记录逐项结论、代码和测试入口。完整自然语言复测报告、原始命令、截图、导出文件及哈希清单作为本地交付档案保留，不随源码公开；文中“本地验收档案”不是远端下载链接。只有在完成隐私审核并实际上传后，才补充可下载的 Release 附件链接。

> 后续状态（2026-11-19）：G1/G2真实双账号样本及单账号对照已完成，最终本地覆盖率达到81.35%，原生Keychain合成凭据往返通过；37.2关联字段经用户确认保留、不阻塞发布，G1–G5列明样本已完成，干净检出覆盖率81.14%已通过，发布仍需实际CI通过。下文测试数及未测描述是该次历史快照，最新结果见[发布前验收补充](release-acceptance-2026-11-19.md)。

2026-11-19，针对 [Issue #1](https://github.com/shimo-office/office-suites/issues/1) 的本地源码修复。原 Issue 基于 f2293db / 0.9.15 静态阅读；修复随后进入 e576fb9 / v0.9.15，Issue #1 已关闭。**本页正文保留提交前修复快照；其后发现的残留与新问题见 [Issue #3 修复记录](issue-3-review.md)，不能将早期“已修复”视为无残留。**

状态区分：前轮已经消除的矛盾按当前源码复核；本轮新增行为修复用失败回归→修复→通过验证。设计选择只写实际边界，不扩大为未实现的功能。验证摘要和源码/二进制指纹保存在本轮证据（本地验收档案）。

## 逐项结论

| 编号 | 结论 | 修复或设计边界 | 代码/证据 |
|---|---|---|---|
| A1 | 属实，前轮已修并复核 | 文本与 JSON 均只报告实际截图；无核验时明确待核验。 | [输出回归](../cli/editor_output_test.go) |
| A2 | 属实，已修复 | save-version/comment-add/print 在执行前拒绝，转持久页面完成；历史/评论显示只声明 panel-visibility-only。保留 --verify 兼容名，明确是显示动作的执行及截图开关。 | [实现](../cli/commands_phase2.go)、[回归](../cli/issue_browser_test.go) |
| A3 | 属实，已修复 | no-browser 与环境变量禁止页面 fetch 启动浏览器，且在目标查询前拒绝；no-verify 只禁截图，不再混同。 | [回归](../cli/issue_browser_test.go) |
| A4 | 属实，前轮已修并新增回归 | help 示例使用带工作表名的 range；测试提取真实 help payload，经命令入口发送到模拟 API 验证参数。 | [契约测试](../cli/issue_contract_test.go) |
| A5 | 属实，已修复 | “可用动作”只列实际映射；需要页面完成的动作单独列为 pageOnlyActions。 | [动作表](../cli/commands_phase2.go)、[回归](../cli/issue_browser_test.go) |
| A8 | 属实，已修复 | 无凭据时显示未登录，credentials 检查失败；不再以“浏览器复用”默认值虚报正常。 | [doctor](../cli/commands_read.go)、[回归](../cli/issue_contract_test.go) |
| A10 | 属实，已修复 | 默认 get/read 按文件类型启用导出兜底，按实际扩展名保存和解析；md 可回退 docx，PDF 无解析器则明确失败，不输出二进制乱码。固定格式验证不降级。 | [文件操作](../cli/commands_file.go)、[9 项回归](../cli/issue_file_regression_test.go) |
| A11 | 属实，已修复 | 改名失败非 0，保留已创建 GUID、URL、真实旧名称、imported:true/renamed:false；上传后失败和所有写命令禁止重放。 | [导入](../cli/commands_write.go)、[回归](../cli/issue_file_regression_test.go) |
| A12 | 属实，已修复 | open 不再注入 cookie 或保留调试端口，明确 sessionInjected:false；不再声称“已用登录态打开”。 | [open](../cli/commands_fav.go)、[假浏览器进程参数回归](../cli/issue_browser_test.go) |
| A13 | 属实，已修复 | init/config set 共用先探测后保存逻辑，只接受 200 JSON、401、403；失败不覆盖旧配置。 | [配置处理](../cli/commands_read.go)、[回归](../cli/issue_contract_test.go) |
| B1 | 属实，文档已统一并复核 | get --format csv 明确拒绝；默认导出与格式兜底另行说明，不声称仍成功导出 XLSX。 | [命令参考](cli-reference.md)、[回归](../cli/issue_file_regression_test.go) |
| B2 | 属实，已统一 | init/config set 使用 GET /api/v1/auth/register/is_new，不以 users/me 判断站点可用；历史 CHANGELOG 保留当时事实。 | [安装说明](install.md)、[实现](../cli/commands_read.go) |
| B3 | 属实，前轮已统一并复核 | WorkBuddy 内置能力与 agent-browser 不混称，教程只引用唯一浏览器规则。 | [浏览器优先级](scenarios.md#浏览器优先级)、[技能](agent/SKILL.md) |
| B4 | 属实，已修复残留 | 八种类型在 help/schema/参考中一致；错误提示将 table 称为应用表格，区别 sheet 电子表格。 | [类型契约](../cli/capability_catalog_test.go)、[回归](../cli/issue_file_regression_test.go) |
| B5 | 属实，已统一 | 实际顺序为环境变量→origin 匹配文件→origin 匹配系统凭据→无已存记录时的浏览器复用；安全文档明确来源与边界。 | [SECURITY](../SECURITY.md)、[凭据实现](../cli/auth.go) |
| B8 | 属实，已修复 | --json 支持命令名前后；未知命令 exit 2 且单个 JSON 错误；doctor/audit/pull 失败只在 stderr 输出一个含明细的 JSON；pull 有失败项非 0。 | [入口](../cli/main.go)、[错误载体](../cli/command_error.go)、[进程级测试](../cli/issue_contract_test.go) |
| B9 | 属实，选择明确当前层语义并修复 | pull 不递归；子目录也进入 manifest，标记 skipped 和原因；输出 recursive:false/scope:current-directory。 | [文件操作](../cli/commands_file.go)、[场景 5](scenarios.md#文件与目录) |
| B10 | 属实，已统一 | --default 选择默认浏览器应用；兼容 Chromium 使用隔离 open-profile，其他交系统打开；不承诺日常浏览器配置或登录态。 | [打开页面](cli-reference.md#打开页面)、[open](../cli/commands_fav.go) |
| C1 | 属实，前轮已统一并复核 | 底色使用 sheet-format attrs.background；不再维护多份相反场景。 | [场景 24/27](scenarios.md#编辑与协作)、[技能](agent/SKILL.md) |
| C2 | 属实，已修复 | 保存版本、评论提交和打印改为持久页面完成；CLI 仅保留历史/评论显示核验，不能把面板打开当作提交。 | [命令参考](cli-reference.md#页面动作)、[能力源](internal/capabilities.json) |
| C3 | 属实，已统一 | 无存储凭据时允许复用；认证失败不静默换号，显式登录才更换会话；移除“绝不能重新登录”的旧规程。 | [登录说明](install.md#登录复用会话与切账号)、[技能](agent/SKILL.md) |
| C4 | 属实，已统一 | 写入完成仍需读回、导出、真实截图；无验证只能待核验。schema verifiedActions 增加含义，表示历史 API 路径记录，不是本次调用已验收。 | [schema](../cli/schema.go)、[核验标准](cli-reference.md#编辑器接口与页面入口) |
| C5 | 属实，前轮规则已统一并修复残留 | 浏览器选择只维护一处；报错不强迫特定宿主直接 open。用户既有授权不重复索取。 | [唯一规则](scenarios.md#浏览器优先级)、[错误提示](../cli/commands_phase2.go) |
| C6 | 属实，前轮已修并复核 | 28 场景、51 条能力分类计数，API 26/页面 24/人工禁用 1，非全部可自动执行；schema 包含 purge，测试由唯一目录验证而非手写命令数。 | [能力源](internal/capabilities.json)、[生成契约](../cli/capability_catalog_test.go) |
| D1 | 属实，已修复 | session harvest 按实际持久化元数据输出 Keychain/凭据管理器/文件，no-save 不声称已保存。 | [会话实现](../cli/session.go)、[安全回归](../cli/auth_security_test.go) |
| D2 | 属实，已修复 | 复用提示改为实际浏览器标签或中性“浏览器”；文档包含支持发现的 Vivaldi，不将其等同所有启动路径支持。 | [会话](../cli/session.go)、[登录](../cli/auth.go) |
| D3 | 属实，已修复 | 登录与复用共用目标主机精确匹配、sid 必需、有效期及 Secure cookie 规则，禁止相似域名匹配和 HTTPS cookie 降级。 | [凭据回归](../cli/auth_security_test.go) |
| D4 | 属实，已统一 | limit 明确为扫描文件数上限，最近更新仅对扫描范围内排序；JSON 增加 limitScope/orderingScope。 | [订阅](../cli/commands_subscription.go)、[参考](cli-reference.md#api文件与目录) |
| D5 | 属实，已修复 | 两个平台安装脚本安装完仅执行 version，不提前触发 doctor 的凭据复用；提示用户随后初始化/登录。 | [安装器](../install.command)、[Windows 安装器](../install.ps1)、[回归](../cli/issue_contract_test.go) |
| D6 | 属实，已修复测试语义 | 原测试改用真实 actions.showViewHistory；命令层白名单由实际可执行映射回归验证，不声称通用表达式函数自行拒绝任意方法。 | [原测试](../cli/commands_phase2_test.go)、[入口测试](../cli/issue_browser_test.go) |
| D7 | 属实，已清理 | 删除无调用的 cookieDomainFor 及错误注释，复用统一 cookie 过滤函数。 | [session](../cli/session.go) |

## 验证与交付边界

本轮验证使用合成凭据、模拟服务端、临时目录/文档和假浏览器进程；不访问客户文件，不切换用户真实账号。涉及下载的测试使用实际生成的 docx/PDF 字节和本机回环服务，不能将这些算作新线上验收。

完整结果由本轮证据清单记录：定向回归、全量 Go 测试、vet/gofmt、生成器和文档检查、报告测试、标准二进制重建、命令进程级退出码及跨平台编译。源码未发布，GitHub Issue 的复选框和状态未改动。前轮 30 项清单日志保留为历史证据；三期的 27 项通过、1 项环境阻塞与 G1–G5/双账号待验收均不因本次修复变为通过。

本轮全量 Go 测试记录 799 个通过事件、0 失败、70 跳过；覆盖率 58.8%，未达到工作区要求的 80%，不能标为发布门槛全部通过。15 项 Python 工具测试、19 项报告测试、47 份 Markdown 检查、vet/gofmt、能力生成一致性及脚本语法检查均通过。重建后的标准 CLI 通过 7 项独立进程冒烟测试。Windows/Linux 完成生产代码和测试二进制交叉编译，未执行这两个系统的原生运行测试；真实 Keychain 写入及线上双账号仍待验收。
