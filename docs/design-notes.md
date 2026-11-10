# 技术选型：CLI 用什么语言

> 结论先行：**如果今天从零开始做，我会选 Go。** 但当前这版继续用 Python 更划算——它已经能跑通 15 个场景，重写的收益要等「Windows / 免 Python 安装 / 单二进制分发」成为硬需求时才兑现。下面是数据和判断依据。

## 1. GitHub 上高星 CLI 的语言分布

用 GitHub API 抓了 4 组查询（`topic:cli`、`topic:command-line-tool`、`topic:command-line`、名称/描述含 CLI），去重后 237 个仓库，按主语言统计：

| 语言 | 数量 | 占比 |
|---|---:|---:|
| Rust | 55 | 23% |
| Go | 46 | 19% |
| Python | 35 | 15% |
| TypeScript | 28 | 12% |
| JavaScript | 22 | 9% |
| Shell | 11 | 5% |
| C++ | 8 | 3% |
| C | 7 | 3% |
| C# | 6 | 3% |
| 其他 | 19 | 8% |

这个样本并不纯：里面混了 TUI 框架（bubbletea、ink）、AI Agent（gemini-cli、aider）、桌面应用和清单仓库。但它能说明一个方向：**真正做成「装一次就能用」的独立 CLI，Go 和 Rust 占主导；Python 更多出现在 API 自动化、数据处理和脚本型工具里。**

代表性项目：

| 类型 | 代表项目 | 语言 |
|---|---|---|
| 单二进制开发工具 | `cli/cli`、`fzf`、`lazygit`、`dive` | Go |
| 高性能文件 / 文本工具 | `ripgrep`、`bat`、`fd`、`zoxide`、`yazi` | Rust |
| API / 自动化脚本 | `httpie`、`yt-dlp`、`sherlock`、`awscli` | Python |
| 前端 / JS 生态 | `gemini-cli`、`vercel`、`nx` | TypeScript |
| macOS 原生 | `cmux` | Swift |

> 数据口径：GitHub Search API，2026-11-10，stars 排序前 100 / 每组；只取仓库主语言，不按代码行数加权。

## 2. 我们的业务逻辑是什么

`shimo-net` 不是纯文本处理工具，它同时做四件事：

1. **HTTP API 客户端**：搜索、目录、导入轮询、导出、上传、权限、回收站；JSON 为主，少量 multipart。
2. **浏览器登录**：启动 Chrome、连 CDP、取 cookie、写 Keychain；只在前端登录环节用浏览器。
3. **本地文件处理**：上传 / 下载、manifest、docx / xlsx / pptx 解包与正文提取。
4. **Agent 接入**：单命令、`--json`、技能描述、可预测的退出码和错误。

使用场景是**办公交付**：使用者是解决方案、销售、交付同学，以及 Agent；机器以 macOS 为主，未来可能扩到 Windows。核心诉求是**装得上、跑得稳、出错能看懂**，不是极致性能。

## 3. 四种语言的匹配度

| 维度 | Python（现状） | Go | Rust | TypeScript / Node |
|---|---|---|---|---|
| 安装门槛 | 需要 Python 3.9+ | 单二进制 | 单二进制 | 需要 Node，或打包 |
| 跨平台 | macOS 好，Windows 弱 | 强（交叉编译） | 强 | 中（打包后体积大） |
| Keychain | 调 `security` | 系统库 / 库 | 库 | `keytar` |
| Chrome CDP | 手写 WebSocket（已有） | `chromedp` 成熟 | 有库 | `puppeteer-core` 最顺 |
| Office 解析 | 标准库 zip/xml（已有） | `excelize` + zip/xml | 生态较少 | 生态最丰富 |
| 迭代速度 | 快 | 中 | 慢 | 快 |
| 团队成本 | 低（代码已存在） | 中 | 高 | 中 |
| 二进制体积 | 需要解释器 | 10–15 MB | 8–12 MB | 40 MB+ |

## 4. 结论

### 今天从零开始：选 Go

- CLI 的交付形态是「下载一个文件就能跑」，Go 的单二进制、交叉编译、`brew` / `scoop` 分发最省事；
- HTTP / JSON / 并发是标准库强项；
- `chromedp` 处理 Chrome 登录，`keyring` 处理 Keychain / Credential Manager；
- Windows 支持不需要另起一套运行时；
- Office 解析可以用 `excelize` + zip/xml，必要时对 docx / pptx 保留 XML 解析。

Rust 也能做到，但开发成本更高，Office 解析生态不如 Go / TS，对这个 CRUD 型 API 客户端属于过度设计。TypeScript 的库最全，但 Node 运行时和打包体积会抵消一部分体验优势。

### 当前这版：继续 Python，做到 0.7

理由：

1. 15 个场景的协议细节已经踩完，重写会把已解决的问题再解一遍；
2. 现在没有登录环境，Go 版本无法做端到端回归，重写风险大于收益；
3. 目标用户当前以 macOS Agent 环境为主，Python 3.9+ 基本可用；
4. 先把「交互流程、README、Agent 接入」做对，比换语言更能提升使用体验。

### 什么时候切 Go

满足任意两条就启动 Go 版本：

- 需要 Windows 客户端，且不能要求预装 Python；
- 需要 Homebrew / Scoop / 签名安装包分发；
- 单文件安装成为销售/交付的硬门槛；
- Python 侧开始为了兼容 Windows 大量打补丁；
- 团队有 Go 维护者，能接住后续迭代。

### 如果切 Go，迁移顺序

1. `init` / `config` / `login` / `whoami` / `doctor`（认证与配置，最关键）；
2. `search` / `spaces` / `ls` / `info`（只读，容易验证）；
3. `import` / `share` / `new` / `rename` / `delete`（写操作，先做只读回归）；
4. `pull` / `get` / `read`（文件与解析）；
5. `fav` / `open` / `schema`（收尾）；
6. 用 Python 版做行为对照，`schema` 作为契约，逐条对齐 JSON 字段和退出码。

> 当前版本：Python 0.7.0，15 个场景 + `init` / `config` / `--relogin`。Go 版本尚未开始，等上面的触发条件。
