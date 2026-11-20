# npm 分发与发布

包名为 `shimo-net-cli`，安装后统一运行 `shimo-net`。本页描述打包及验收流程；生成 `.tgz`、上传 GitHub Release 与发布到 npm 是三个不同步骤，不能互相替代。是否已上架及具体版本须以官方 npm registry 查询结果为准。

## 为什么另加 npm

技能 ZIP 中的旧启动器首次需要从 GitHub Releases 下载 CLI，网络不可达时无法启动。npm 分发把 macOS、Linux、Windows 各自 amd64 / arm64 的六个完整程序直接装入同一个包；安装和运行都不再通过脚本转去 GitHub 下载程序。

这个方案用较大的 npm 安装包换取完整、自包含的分发和单包版本管理。npm registry 仍可能遇到网络问题，不能保证永不超时；访问石墨文件仍需要目标服务可达和用户登录。Homebrew 只是一种安装方式：若公式依旧指向 GitHub Releases，下载依赖并未改变。

npm 入口需要 Node.js 18+，建议使用受支持的 Node.js LTS。原 Go CLI 和直接二进制安装方式不新增 Node 依赖。包内没有第三方 npm 依赖，没有 `postinstall` 等生命周期脚本；启动器验证包内 SHA-256 后执行当前系统的原生程序，不拼接 shell 命令。

## 用户安装

在指定版本已经发布到 npm 后，运行：

```bash
npm install --global shimo-net-cli@<版本> --registry=https://registry.npmjs.org
shimo-net version
shimo-net --help
shimo-net schema --json
```

无全局目录写权限时，可使用用户拥有的 npm prefix，或在项目中执行 `npm install shimo-net-cli@<版本>` 后通过 `npm exec -- shimo-net version` 调用；不要通过关闭系统安全保护解决安装问题。

随后按[安装与登录说明](install.md)运行 `doctor`、登录和真实文件检查。无登录状态下 `doctor` 非零退出不等于 npm 安装失败；帮助和版本正常也不等于业务功能已验收。

## 维护者构建与验收

`scripts/package_npm.py` 使用与 Release 相同的六个二进制，校验 CLI、技能、标签版本一致，只打包白名单文件：包元数据、说明、许可证、启动器、六个二进制及 SHA-256 清单。它不会读取或打包账号配置、Cookie、`.npmrc` 或工作目录中的其他文件。

```bash
python3 scripts/package_npm.py --tag v<版本> --assets-dir dist --out-dir dist
npm install --global --prefix <隔离安装目录> --cache <隔离缓存目录> \
  --offline --ignore-scripts --no-audit --no-fund ./dist/shimo-net-cli-<版本>.tgz
```

通过隔离目录安装出的命令执行版本、帮助、schema；再核对参数传递、退出码、错误路径与目标系统兼容性。Unix 的命令在 `<隔离安装目录>/bin/shimo-net`；Windows 在 `<隔离安装目录>/shimo-net.cmd`。使用当前系统真实二进制执行，不能用跨平台包结构检查冒充 Windows/Linux 实际运行。

Release workflow 自动生成 `.tgz` 附件；目前不自动发布 npm。这个附件用于 npm 安装/发布，不能上传到要求技能 ZIP 的 SkillHub 或 WorkBuddy 表单。

## 首次发布账号操作

1. 维护者在 [npm 官网](https://www.npmjs.com/signup)注册或登录自己控制的账号，完成邮箱验证，并按 npm 当前提示配置双重验证。
2. 在自己的终端执行 `npm login --registry=https://registry.npmjs.org`，按浏览器提示完成登录。无需在聊天中发送密码、验证码或 token。
3. 用 `npm whoami --registry=https://registry.npmjs.org` 检查当前身份；确认其拥有待发布包的权限。未注册包名能否占用仍以发布时 registry 校验为准。
4. 检查包内文件与真实安装验收结果，再运行以下命令。交互式验证由账号持有人完成，不把凭据写入仓库。

```bash
npm publish ./dist/shimo-net-cli-<版本>.tgz --access public --registry=https://registry.npmjs.org
npm view shimo-net-cli@<版本> version dist.integrity --registry=https://registry.npmjs.org
```

同一版本不能覆写。修改代码或二进制应递增版本。发布后还需从官方 npm registry 重新安装、执行版本及帮助，确认消费者拿到的产物与待发布包一致。

官方参考：[npm 包元数据与命令入口](https://docs.npmjs.com/cli/v11/configuring-npm/package-json)、[npm publish](https://docs.npmjs.com/cli/v11/commands/npm-publish)。
