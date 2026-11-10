# Changelog

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
