# 参与贡献

`shimo-net` 是石墨办公套件的私有部署 CLI。本仓库的源码用于公开分发与问题反馈，**不采用开源许可**：复制、修改、分发或商业使用须事先获得 shimo-office 团队书面授权，条款见 [LICENSE](LICENSE)。代码贡献以邀请或事先沟通为前提，未经沟通的 PR 可能无法合并。

## 反馈问题

- 功能异常、文档错误：用 [Issue 模板](https://github.com/shimo-office/office-suites/issues/new/choose) 提交，附 `shimo-net version --json`、操作系统、复现步骤和脱敏后的错误输出。
- 安全漏洞：不要开公开 Issue，按 [SECURITY.md](SECURITY.md) 的方式私下报告。
- 使用问题：先查[安装与排错](docs/install.md#faq)和[命令参考](docs/cli-reference.md)。

## 开发环境

| 依赖 | 版本 | 用途 |
|---|---|---|
| Go | 1.26.11 | CLI 主体，交叉编译产出六个平台的二进制 |
| Python | 3.9+ | 能力目录生成、打包与发布校验脚本 |
| Node.js | 18+ | 仅前端回归测试，CLI 运行时不依赖 |

## 本地检查

提交前跑一遍与 CI 等价的检查：

```bash
make test    # 生成目录校验、Python 单测、构建与基础冒烟
make lint    # 文档检查、Python 单测、go vet
```

CI 另外要求：

```bash
go test ./cli -coverprofile=coverage.out
python3 scripts/check_coverage.py coverage.out   # 语句覆盖率 ≥ 80%
go run golang.org/x/vuln/cmd/govulncheck@v1.10.0 ./cli
```

Windows / Linux 两个平台都会跑全量测试，改动涉及路径、凭据或进程行为时请在两个平台都验证。

## 提交信息

使用 [Conventional Commits](https://www.conventionalcommits.org/)：`feat` / `fix` / `docs` / `ci` / `chore` / `test` / `refactor`。标题写清「改了什么」，正文补充影响面与验证方式；涉及 Issue 或 PR 时带上编号。

仓库的主线历史是**一次发布一条提交**，每条提交对应一个 `vX.Y.Z` 标签，正文即该版本的变更说明。合并前的分支提交可以细碎，合入 `main` 时请整理成一条与发布对应的提交。

## 分支与发布

1. 从 `main` 拉分支开发，通过 PR 合入，保持 `main` 可发布。
2. 合入后更新 `CHANGELOG.md`，并把两处版本号对齐：`cli/main.go` 的 `version` 常量与 `docs/agent/SKILL.md` 的 `version` 字段。打包脚本会校验两者与标签一致，不一致直接失败。
3. 在 `main` 的最新提交上打 `vX.Y.Z` 标签并推送。标签触发的 `Release` 工作流会：
   - 校验标签必须位于 `main`；
   - 跑与 CI 相同的测试与覆盖率门槛；
   - 交叉编译六平台二进制，生成 SkillHub / WorkBuddy 技能包与 npm 包；
   - 上传资产并逐项比对 SHA-256，随后发布 npm，并将分支与标签镜像到 Gitee。

标签推送失败或被门槛拦截时，修好问题后重新打标签；不要复用已发布的版本号。

## 文档

命令行为、参数或输出发生变化时，同步更新 `docs/` 下对应页面与 `CHANGELOG.md`。能力清单一类可生成的表格由 `scripts/generate_capabilities.py` 产出，不要手改：

```bash
python3 scripts/generate_capabilities.py          # 重新生成
python3 scripts/generate_capabilities.py --check  # 校验是否漂移
```
