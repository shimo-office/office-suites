## 改了什么

<!-- 一句话说明这次变更的目的，以及它解决的问题。 -->

## 变更类型

- [ ] `feat` 新能力或新场景
- [ ] `fix` 行为修正
- [ ] `docs` 文档
- [ ] `ci` / `chore` 构建、发布或仓库配置
- [ ] `test` 测试

## 影响面

<!-- 涉及的命令、参数、输出格式或平台。无用户可见变化时写「无」。 -->

## 验证

<!-- 贴出实际执行的命令与结果，例如 make test、go test ./cli、Windows/Linux 实测。不要只写「已测试」。 -->

```bash
make test
```

## 检查清单

- [ ] `make test` 与 `make lint` 通过
- [ ] 改到命令行为时，同步了 `docs/` 与 `CHANGELOG.md`
- [ ] 新增或修改的能力已被 `scripts/generate_capabilities.py --check` 覆盖
- [ ] 提交信息符合 [Conventional Commits](CONTRIBUTING.md#提交信息)
- [ ] 涉及凭据、路径或进程行为时，已在 Windows 与 Linux 上验证
- [ ] 未包含账号、cookie、内网地址等敏感信息
