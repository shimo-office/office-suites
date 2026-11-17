# WorkBuddy × 石墨办公：从打开到第一个结果

> 适用版本：WorkBuddy 5.9.3（桌面客户端）· shimo-net CLI 最新 release
> 适用对象：shimo.net 客户及私有化部署客户。照着做，20 分钟内完成「打开 Agent → 安装市场技能 → 装 CLI → 登录授权 → 跑出第一个真实结果」。

## 0. 开始前

你只需要准备：

- 已经安装好的 WorkBuddy 桌面端（不需要重新下载）；
- WorkBuddy 市场中的 [shimo-net 技能](https://www.workbuddy.cn/share/asset?extra_from=&resource_type=link&shareCode=cs261116r5dG5qMyyGQqvMFkG0pjYT&share_way=link&sig=lXMg0jOdaNQCDJnv)；
- 一个能登录 shimo.net 的账号；
- macOS 的「终端」，或 Windows 的 PowerShell；
- 能访问 GitHub（安装 CLI 用）。

不需要会写代码；命令都可以直接复制。

## 0.1 一句话让 Agent 自己配置（推荐）

把下面整段发给 WorkBuddy，不要自己开终端：

```text
请直接使用已经安装的 WorkBuddy 官方市场版 shimo-net 技能完成配置，不要上传或复制仓库里的 cli/SKILL.md，也不要重复安装技能：
1. 先检查本机 shimo-net CLI 是否可用；如果没有，再从 GitHub Releases 的 latest 版本安装，不要固定旧版本号。
2. 如果需要登录，打开 Chrome 登录页；我会完成登录，登录成功后继续。
3. 用 ~/.local/bin/shimo-net whoami --json 验证，只回复「已配置完成，账号可用」，不要显示账号信息。
如果配置失败，返回完整错误信息。
```

市场技能安装后可以直接使用；只有本机没有 CLI 时才下载 latest 版本并初始化。登录完成后，如果「已安装技能」没刷新，退出 WorkBuddy 再打开一次；如果它问是否允许执行命令，选允许。

> CLI 安装脚本公开可审计：https://github.com/shimo-office/office-suites/blob/main/bootstrap.sh；脚本默认从 GitHub Releases latest 下载。
> 如果 Agent 不执行命令或网络失败，再按下面的手工步骤做。

真实执行结果（WorkBuddy 5.9.3，38 秒）：

![WorkBuddy 一句话自配置](images/workbuddy/real-self-setup.jpg)

## 1. 打开 WorkBuddy

从「应用程序」打开 WorkBuddy，确认能看到任务输入框。看到「WorkBuddy，我帮你」和输入框就对了；这张空白首页不用截图，直接进入下一步。

## 2. 检查 shimo-net CLI（已安装可跳过）

市场技能安装后先直接使用。如果 WorkBuddy 提示本机没有 `shimo-net`，再打开「终端」安装 GitHub Releases 的 latest 版本：

```bash
curl -fsSL https://github.com/shimo-office/office-suites/releases/latest/download/bootstrap.sh | bash -s -- workbuddy
shimo-net version
shimo-net doctor --json
```

Windows 用 PowerShell 时，下载仓库后执行安装器，或直接安装当前 release 二进制：

```powershell
git clone https://github.com/shimo-office/office-suites.git
cd office-suites
powershell -ExecutionPolicy Bypass -File .\install.ps1
shimo-net version
shimo-net doctor --json
```

正常情况下会看到版本号和配置信息：

![CLI 安装与配置验证](images/common/cli-version.jpg)

你要确认三件事（如果 CLI 已存在，只需确认版本和 doctor）：

1. `shimo-net version` 输出当前安装的 latest release 版本；
2. `shimo-net config show` 显示 `https://shimo.net`；
3. `doctor --json` 的 `ok` 为 `true`，`checks` 五项都是 `ok`；终端最后一行显示 `结果：🟢 doctor: OK`。

装完后如果提示 `shimo-net: command not found`，先执行：

```bash
export PATH="$HOME/.local/bin:$PATH"
```

想永久生效，把这一行加到 `~/.zshrc` 末尾，再执行 `source ~/.zshrc`。Windows 关掉 PowerShell 重新打开一次，安装脚本会写入用户 PATH。

## 3. 登录授权

在终端执行：

```bash
shimo-net login
```

会弹出一个 Chrome 窗口：

1. 用你的 shimo.net 账号登录；
2. 看到登录成功提示后，Chrome 会自动关闭；
3. 回到终端，执行：

```bash
shimo-net whoami --json
```

返回账号信息就代表授权完成。CLI 会把会话存进 macOS Keychain（Windows 是凭据管理器），**下次不用再登录**。

> 登录页会显示你的账号，教程不截这张图；用 `whoami` 的返回结果作为授权验收即可。

如果返回「未登录」：

```bash
shimo-net logout
shimo-net login
shimo-net whoami --json
```

## 4. 安装 WorkBuddy 市场技能

WorkBuddy 已经提供官方 shimo-net 市场技能，不需要上传或复制仓库里的 `cli/SKILL.md`。

1. 打开 [WorkBuddy 市场技能](https://www.workbuddy.cn/share/asset?extra_from=&resource_type=link&shareCode=cs261116r5dG5qMyyGQqvMFkG0pjYT&share_way=link&sig=lXMg0jOdaNQCDJnv)；
2. 点击「安装」或「添加到 WorkBuddy」；
3. 回到左侧栏「专家 · 技能 · 连接器」→「技能」→「已安装」；
4. 确认市场版 shimo-net 技能已启用。

装好后打开「已安装技能」，应该看到：

![WorkBuddy 已安装技能](images/workbuddy/real-installed.jpg)

### 启用内置浏览器

没有 API 的页面操作（例如文字/段落格式、历史、评论添加、表格冻结排序和打印）需要 WorkBuddy 的网页操作能力。请在「专家 · 技能 · 连接器 → 技能 → 已安装」确认官方 **agent-browser** 技能已安装并打开开关。它是 WorkBuddy 的内置浏览器能力，负责打开页面、点击菜单、输入内容和截图。

Agent 执行页面操作时遵循以下顺序：

1. 已启用 `agent-browser`：直接在 WorkBuddy 内置浏览器打开石墨文件并操作，不能先运行 `shimo-net open`。
2. 未安装或未启用：先询问「是否允许我安装并启用 WorkBuddy 市场的官方 agent-browser 技能？」；用户允许后再安装/启用并验证一次打开、点击和截图能力，完成后继续任务。技能已经安装时只开启开关，不重复安装。
3. 用户允许但宿主确认没有内置浏览器：才运行 `shimo-net open <链接> --default` 调起本机默认浏览器。CLI 会尝试注入本机 Keychain 登录态；不能安全注入时如实提示并保留页面结果。用户拒绝安装/启用时，不要擅自弹出外部浏览器。

`shimo-net` 无法从命令行强制创建 WorkBuddy 内置浏览器，也不能把仓库里的自定义 `cli/SKILL.md` 当作 Computer Use 安装。内置浏览器是 WorkBuddy 的宿主技能，市场技能安装并启用后才可用。

判断标准是市场版 shimo-net 技能出现在「已安装」并处于启用状态；不需要检查 `~/.workbuddy/skills/`，也不要安装仓库自定义技能。

装好之后不需要每次手动挑技能：在任务里直接写「用 shimo-net …」，Agent 会按技能描述自动调用。后面的任务都是这种方式。

## 5. 第一个任务：验证连通

回到 WorkBuddy，点「新建任务」，在输入框里粘贴：

```text
用 shimo-net 执行 whoami --json 验证连通；只回复「已连通，账号可用」，不要显示账号信息。
```

点发送，等 10–20 秒。你应该看到：

![WorkBuddy 执行 whoami](images/workbuddy/real-whoami.jpg)

出现「已连通，账号可用」就说明：

- WorkBuddy 能执行本机命令；
- `shimo-net` 在 PATH 里；
- 登录态有效；
- 技能已经被调用。

如果没有执行命令，先检查第 4 步的技能是否启用，再检查 WorkBuddy 是否允许「执行本地命令 / 访问工作目录」。

## 6. 第二个任务：跑一个真实操作

连通只说明「能调用」，再跑一个真实操作，确认结果能落地。

在 WorkBuddy 新任务里粘贴（把链接换成你有权限的文档）：

```text
用 shimo-net info https://shimo.net/docs/MWn7L07R8jhPEm1p 返回名称、类型、链接；只回复这三项，不要显示账号信息。
```

你应该看到文档名称、类型和链接：

![WorkBuddy 执行 info](images/workbuddy/real-info.jpg)

看到「名称 / 类型 / 链接」三项，并且链接能打开，就完成了从授权到操作结果的闭环。

> 截图里的「（图文教程自检）」是演示文档；你实际使用时换成自己的文档链接。

## 7. 当前场景怎么用

流程和上面完全一样：在 WorkBuddy 里说人话，Agent 会调用 `shimo-net`。下面是可以直接复制的任务说法。

| # | 场景 | 在 WorkBuddy 里说 | 对应命令 |
|---|---|---|---|
| 01 | 找文件 | 帮我找一下「季度方案」，最多 5 条 | `shimo-net search "季度方案" --limit 5 --json` |
| 02 | 看目录 | 看看这个文件夹里有什么：<链接> | `shimo-net ls <链接> --json` |
| 03 | 文件出处 | 这份文档在哪个空间、谁有权限：<链接> | `shimo-net info <链接> --json` |
| 04 | 导入并设权限 | 把这份 docx 导到「交付」文件夹，权限到石墨网页人工设置 | `shimo-net import ./file.docx --to <文件夹> --json`，再打开页面人工操作 |
| 05 | 拉取文件夹 | 把这个文件夹拉到本地 ./交付 | `shimo-net pull <链接> --out ./交付 --json` |
| 06 | 新建文档 | 在桌面新建一个文档，叫「会议纪要」 | `shimo-net new "会议纪要" --type doc --to desktop --json` |
| 07 | 改名 | 把「会议纪要」改名为「周会纪要」 | `shimo-net rename <链接> "周会纪要" --json` |
| 08 | 删除/恢复 | 把这份文档移到回收站；再恢复 | `shimo-net delete <链接>` / `shimo-net restore <guid>` |
| 09 | 权限巡检 | 看看这个文件夹里哪些是公开的 | `shimo-net audit <文件夹> --json` |
| 10 | 批量改权限 | 把这个文件夹里的文档都改成「组织内可读」 | 先 `audit`，再打开石墨网页逐项人工确认和设置 |
| 11 | 读正文 | 读一下这份文档，给我 5 条摘要 | `shimo-net read <链接> --max-chars 12000` |
| 12 | 周报 | 把这个文件夹最近 7 天的内容整理成一页纸 | `shimo-net ls <链接> --since 7d` + `shimo-net read` |
| 13 | 下载附件 | 把这份文档的附件下载到 ./附件 | `shimo-net get <链接> --out ./附件` |
| 14 | 上传 | 把这个 PDF 传到「资料」文件夹，先私有 | `shimo-net upload ./file.pdf --to <文件夹>` |
| 15 | 常用别名 | 以后把「交付」文件夹叫「交付」 | `shimo-net fav add 交付 <链接> --note 交付目录` |
| 16 | 只看与我相关 | 看看谁 @ 了我、有哪些评论 | `shimo-net phase2 <链接> --action mentions-list|comments-list --json` |
| 17 | 留痕与版本 | 查看历史、保存版本 | `shimo-net open <链接>` 后由 computer-use 操作历史面板 |
| 18 | 文字与段落 | 查找替换、调整文字和段落格式 | `shimo-net open <链接>` 后由 computer-use 操作菜单和选区 |
| 19 | 文内表格与图片 | 插入或调整文内表格、图片 | `shimo-net open <链接>` 后由 computer-use 操作 |
| 20 | 评论与提及 | 读取评论或在页面添加评论/@ | `comments-list` 读取；添加由页面操作 |
| 21 | 预览与打印 | 打开预览或打印 | `shimo-net open <链接>` 后由 computer-use 操作 |
| 22 | 文档重排 | 调整标题层级、页面宽度 | `shimo-net open <链接>` 后由 computer-use 操作 |
| 23 | 表格写入 | 写入或追加单元格区域 | `shimo-net phase2 <链接> --action sheet-update|sheet-append --payload … --json` |
| 24 | 表格整理 | 冻结、排序、筛选 | `shimo-net open <链接>` 后由 computer-use 操作 |
| 25 | 表格清洗 | 去重、分列、填充 | `shimo-net open <链接>` 后由 computer-use 操作 |
| 26 | 表格算与看数 | 公式、名称、透视表 | `shimo-net open <链接>` 后由 computer-use 操作 |
| 27 | 表格校验与格式 | 设置底色、字体、边框、合并 | `shimo-net phase2 <链接> --action sheet-format|sheet-merge --payload … --json` |
| 28 | 表格输出 | 导出文件 | `shimo-net get <链接> --out <目录> --json` |

> 导入、上传等 CLI 写操作执行前，Agent 会先确认位置；文件锁定、限制编辑、高级权限、成员级授权和彻底删除必须打开石墨网页人工处理，删除命令只进回收站。

表格样式示例（直接修改在线表格，不生成本地 XLSX）：

```bash
shimo-net phase2 <表格链接> --action sheet-format \
  --payload '{"sheetName":"工作表1","range":"A1:C1","attrs":{"background":"#D32F2F","color":"#FFFFFF","bold":true}}' --json
shimo-net phase2 <表格链接> --action sheet-merge \
  --payload '{"sheetName":"工作表1","cell":"A1","rowspan":1,"colspan":3}' --json
```

如果某项页面功能没有对应 API，Agent 应先确认 `agent-browser` 已启用，然后打开 WorkBuddy 内置浏览器并用网页操作完成；未启用时先询问用户是否允许安装/启用，获得允许后再继续。用户拒绝且宿主没有内置浏览器时，停止页面操作；用户允许但宿主仍无内置浏览器时才使用 `shimo-net open <链接> --default`。CLI 会复用 Keychain 会话，不输出 sid/cookie。

## 8. 验收清单

全部打勾，才算独立完成：

- [ ] WorkBuddy 能正常打开，能看到任务输入框；
- [ ] `shimo-net version` 输出当前安装的 latest release 版本；
- [ ] `shimo-net config show` 显示 `https://shimo.net`；
- [ ] `shimo-net whoami --json` 返回账号；
- [ ] WorkBuddy「已安装」里能看到并启用官方市场版 shimo-net 技能；
- [ ] 第一个任务返回「已连通，账号可用」；
- [ ] 第二个任务返回「名称 / 类型 / 链接」三项；
- [ ] 当前场景里至少完整跑通 1 个（建议 01 找文件、03 文件出处、11 读正文）。

## 9. 常见问题

**Q：WorkBuddy 说没有权限执行命令？**  
在 WorkBuddy 的任务设置里允许「执行本地命令 / 读写工作目录」，然后重新发一次任务。

**Q：`shimo-net: command not found`？**  
执行 `export PATH="$HOME/.local/bin:$PATH"`，或重新打开终端；Windows 重新打开 PowerShell。还不行就回到第 2 步重跑 `./install.command`。

**Q：`whoami` 返回「未登录」？**  
执行 `shimo-net login`，完成 Chrome 登录后重试。不要重复执行 `init`，网址已经记住。

**Q：技能装了但 Agent 不调用？**  
确认官方市场版 shimo-net 技能已安装并启用，退出 WorkBuddy 再打开；或者直接在任务里写「用 shimo-net 执行 …」，明确点名命令。

**Q：Windows 上 Keychain 是什么？**  
Windows 用「凭据管理器」，CLI 会自动存到 `shimo-net-cli` 这个目标下，不需要手动打开。

**Q：截图里的链接打不开？**  
那是演示文档，可能已被删除；换成你自己的文档链接即可，命令格式不变。
