# Issue #3：回归残留与新问题修复

对应 [Issue #3](https://github.com/shimo-office/office-suites/issues/3)，源码版本 0.9.18。报告基线为 v0.9.15 / e576fb9，作者后续评论确认 v0.9.17 仍有残留。本次逐项检查 C5、D4 和 N1–N13；不把既有平台审核、历史线上验收或跨平台编译当作本轮真实运行。

## 逐项处理

| 项目 | 处理与边界 | 验证入口 |
|---|---|---|
| C5 | 浏览器规则统一覆盖 open/default、verify、评论/提及、登录/会话复用及三期页面动作；已有授权不重复询问，拒绝后不换工具绕过。订阅输出先给链接，打开提示明确授权条件。 | [浏览器规则](scenarios.md#浏览器优先级)、[订阅实现](../cli/commands_subscription.go) |
| D4、N10 | 订阅扫描按唯一文件计数，目录不消耗文件预算；逐页遍历，返回 scanned/truncated/partial/complete/failures。部分失败保留结果、非零退出，禁止以部分排序结果继续自动打开文件。 | [分页与参数回归](../cli/issue3_parser_subscription_test.go) |
| N1 | 帮助在命令分派及配置读取前处理；未知、错用和缺值选项返回用法错误，不执行原操作。`--` 后参数按字面量处理。 | [参数回归](../cli/issue3_parser_subscription_test.go) |
| N2 | editor 写请求或后续页面核验失败保留 mutationAttempted、mayHaveApplied、noReplay、阶段和 GUID。格式分步失败返回已受理属性及 failedAttribute；不把受理等同最终生效。 | [写入安全回归](../cli/issue3_write_safety_test.go) |
| N3 | new/mkdir 已创建后读回失败返回 created:true、GUID 和禁止重放提示，避免误重建。 | [写入安全回归](../cli/issue3_write_safety_test.go) |
| N4 | login --origin 与 init/config 共用探测后保存逻辑；HTTP/网络失败不覆盖旧配置，不进入登录。 | [认证回归](../cli/issue3_auth_browser_test.go) |
| N5 | 本轮使用 0.9.18；明确 f2293db 与 e576fb9 历史上都自报 0.9.15，必须结合提交/哈希区分。修正旧“未发布/未关闭”文字，不覆写历史标签。 | [来源记录](internal/release-provenance.md)、[Issue #1 后续状态](issue-1-review.md) |
| N6 | schema、场景和回归索引统一为已验收列明范围、37.2 保留；历史 81.35%/81.14% 不冒充新构建覆盖率。 | [schema 回归](../cli/schema_test.go)、[阶段记录](phase3-acceptance.md) |
| N7 | 三期 XLSX/DOCX/PPTX 验证导出禁用格式兜底；普通下载保留原设计的回退，失败证据不再多导一份 PDF。 | [写入与导出回归](../cli/issue3_write_safety_test.go) |
| N8 | 发现浏览器共用平台候选表，补齐 Linux Edge、Windows Chromium；显示实际名称。Windows 默认 HTTPS 处理程序可识别时仅执行浏览器路径及受控隔离参数，不执行注册命令中的附加参数。 | [浏览器回归](../cli/issue3_auth_browser_test.go) |
| N9 | 会话读取、注入、刷新失败及时返回对应阶段；CDP 原始错误可能含凭据，不将其照抄输出。 | [会话注入回归](../cli/issue3_write_safety_test.go) |
| N11 | upload 成功响应必须为对象且包含非空字符串 GUID；异常响应报告可能已上传并禁止自动重放，不显示“上传完成”。 | [上传回归](../cli/issue3_write_safety_test.go) |
| N12 | 合成父域 sid 测试确认会被拒绝；新增即时诊断，不等待十分钟。保持精确主机安全边界，未宣称支持父域 cookie 登录，也未对客户私有环境实测。 | [认证回归](../cli/issue3_auth_browser_test.go) |
| N13 | schema 说明 0600 文件回退，更新会话旧注释及 Vivaldi 列表；仓库简介已通过 GitHub API 更新为 28 个通用场景、51 项能力，并读回核对。 | [schema 回归](../cli/schema_test.go)、[会话说明](install.md) |

## 验证与发布状态

本轮新增回归先复现失败，再验证修复。最终隔离全量 Go 测试：1746 个 pass 事件（含子用例）、0 fail、71 skip，包级通过，耗时 302.9 秒；语句覆盖率 81.6%（Go 四舍五入输出），通过 80% 门槛。Python 工具测试 57 项：50 通过、7 个 Windows 专用项在本机跳过。vet、文档/生成物检查、diff 检查、独立代码与安全审查通过；govulncheck 退出 0、未发现可达漏洞。跳过项不计通过。

六平台构建成功；macOS amd64 与 arm64 的实际 CLI/npm 安装入口均已运行。真实二进制帮助/未知选项/禁止浏览器的 9 项检查保持合成状态逐字节不变；npm 包空缓存离线安装与工作区局部安装通过，无 GitHub 下载。npm 启动器的参数、标准流、退出码、信号及损坏拒绝有执行测试。源码/产物哈希和原始日志保存在本地 output/issue3 档案，不随源码公开。

远端发布状态以 [v0.9.18 Release](https://github.com/shimo-office/office-suites/releases/tag/v0.9.18)、同提交 Actions 和 Issue 关闭评论为准；本页记录本地冻结验收，不能仅凭文档认为 CI、npm 上架或平台审核已经完成。

测试使用独立状态目录、测试凭据服务标识、合成 HTTP/CDP 响应及受控子进程。真实二进制的帮助/未知参数可在空状态目录验证无文件变动；不会用生产文件制造失败。模拟服务的故障注入证明错误与重放处理，不等同重新执行全部线上办公场景。Windows 注册表解析/候选路径单测与交叉编译不等同原生桌面浏览器验收。

## 测试隔离事件

第一轮 RED 测试有一条 `logout --typo` 漏设临时状态目录，触发了旧版退出路径。该路径可能清理本机 CLI 凭据及旧 chrome-profile，不操作云端文件。没有运行前基线，不能确认用户本机凭据是否受影响；已告知用户，未尝试猜测恢复或重新登录。测试随即改为隔离状态并先使用只读命令复现未知参数缺陷；后续集中回归设置独立凭据标识与禁用会话复用。另一次三期禁用浏览器的 RED 用例遗漏了提前执行的协作门禁，曾启动临时 about:blank 窗口，发生在凭据注入和业务导航前；已改为替换全部浏览器构造入口并断言零调用，没有执行无法准确归属的进程清理。

## 分发入口

用户命令与技能示例统一为 shimo-net。仓库 ./shimo 仅保留为旧开发兼容入口。npm 自包含包及首次发布步骤见 [npm 分发说明](npm.md)；生成 npm 产物不代表已经上架，SkillHub/WorkBuddy 页面解析也不代表审核或业务验收通过。
