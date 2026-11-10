# Changelog

## 0.8.0 — 2026-11-10

### 变更

- CLI 重写为 **Go 单二进制**（Go 1.22+），运行时不再依赖 Python。
- 15 个场景、`init` / `config` / `login --origin` / `--relogin` 行为与 0.7.0 对齐。
- Chrome 登录改为 Go 实现的 CDP WebSocket 客户端；会话仍写 macOS Keychain。
- docx / xlsx / pptx 正文解析改为 Go 标准库 zip + xml。
- 安装器：有 Go 时本地构建，否则下载 release 二进制。
- CI 改为 Go 测试；新增 tag 触发的 macOS / Linux 交叉编译与 release 资产。

### 移除

- Python 实现与 `~/.shimo-net-py` 安装目录（历史版本仍可从 git 记录获取）。

## 0.7.0 — 2026-11-10

### 新增

- `init`：首次配置产品网址，`GET /api/v1/users/me` 校验可达后写入 `config.json`。
- `config show / set / clear`：查看、修改、清除产品网址；`SHIMO_NET_ORIGIN` 仍可临时覆盖。
- `login --origin <url>`：首次登录可直接指定私有部署域名；无配置时也会提示输入。
- `--relogin`：遇 401 / 未登录时自动打开 Chrome 登录，成功后重试原命令。
- 根 README 重写为面向使用者的版本：30 秒上手、常见任务、Agent 接入、私有部署、排错。
- 终端演示图 `docs/images/demo.png`。

### 变更

- 版本号 0.7.0；`doctor` 增加 `originSource` / `configFile`。
- `schema` 增加 `init` / `config` 命令与 `configFile` 字段。
- 未知命令退出码为 2；`--version` 可用。
- 安装器 Python 版本检查统一为 3.9。

## 0.6.0 — 2026-11-10

首个公开仓库版本。

### 新增

- 15 个办公场景：搜索、空间 / 目录、文件出处、导入 + 权限、拉取、新建、改名、删除 / 回收站 / 恢复、权限巡检、批量改权限、读正文、周报、附件下载、上传、常用文件夹别名。
- 单文件 Python 实现（`cli/bin/shimo.py`），零第三方依赖，Python 3.9+。
- Chrome 专用窗口登录，macOS Keychain 保存会话；支持 `SHIMO_NET_COOKIE` 与 0600 文件回退。
- `SHIMO_NET_ORIGIN` 切换客户私有部署域名。
- 全局 `--json`，统一成功 / 失败输出与退出码。
- `schema` / `doctor` / `whoami` / `open` 等辅助命令。
- `@别名` 常用文件夹，任意接受链接 / guid 的参数都可使用。
- 导入与上传分离：`import` 转在线文档，`upload` 原样云文件。
- 删除只进回收站；`purge` 明确拒绝，彻底删除需网页手动操作。
- Agent 技能定义 `cli/SKILL.md`。

### 文档与测试

- 根 README：环境要求、命令总览、私有化、安全设计、开发与限制。
- `cli/README.md`、`cli/安装使用说明.md`、`cli/scenarios.md`。
- `cli/tests/smoke.sh` 无网络冒烟测试；`make test`。
- 安全策略 `SECURITY.md`；仓库忽略规则与编辑器配置。

### 兼容性

- macOS 为主支持平台。
- Linux 实验性支持：新增 Chrome / Chromium 常见路径；Keychain 不可用时回退到 0600 文件。
