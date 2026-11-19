# 文稿已实现动作参数（实现 ≠ 全部线上验收通过）

全部使用 `phase3 <docxURL> --action <action> --payload '<JSON>' --out <新证据目录> --json`。原文件编辑；未知在线协作名单/其他用户在线/权限未确认即拒绝。后写验收失败不自动重放。下列动作的完整线上验收状态见 [验收记录](phase3-acceptance.md)。

## document-format

```json
{"paragraph":1,"text":"测试 x2 H2O","match":"x2","offset":1,"length":1,"attrs":{"verticalAlign":"superscript"}}
```

paragraph从1开始；text精确原段落且非空不含换行；match可选但必须唯一；offset/length可选且须成对，为match内（无match时为text内）的Unicode码点位置。attrs：fontFamily（字符串）、fontSize（1–400磅，0.5步长）、color（#RRGGBB）、verticalAlign（baseline/superscript/subscript）、bold/italic/underline/strike（布尔）。已符合请求返回无修改错误，不冒充本次编辑。保存与刷新后的 DOCX 会逐字核对请求格式、全部包资源及正文/表格/分节结构，并保留图形、超链接和未知节点的原位置；仅允许常规文字 run 拆分。未知混合 XML 无法可靠证明位置时直接拒绝，不声称已支持任意复杂格式。

## document-table

```json
{"operation":"insert-row-after","table":1,"row":1,"column":1,"expectedCell":"P3_A1"}
```

operation：insert-row-before、insert-row-after、delete-row、insert-column-before、insert-column-after、delete-column，以及distribute-rows/distribute-columns。上述操作只接受非嵌套、无合并的矩形表格。行列插删仍要求每格单段落，禁止删除最后行或列隐式删整表。均分允许非合并单元格含多个段落，expectedCell 必须使用该格完整文本、多段落以`\n`连接；定位使用首段，写前逐段核对真实段落身份及全文，并在选区回调内同步重验原生 run 文本，无法可靠读取或复杂 run 不受支持时写前拒绝。均分列要求内容最小宽度不超过均分宽度，否则拒绝，不擅自切换布局模式。table/row/column从1开始，expectedCell精确锚点单元格全文。已有2×2专用测试文稿的前后插删行列和均分行列补充证据，含保存、刷新、DOCX 导出及截图核对；原始失败和独立只读恢复分别记录，见[验收记录](phase3-acceptance.md)。

横向合并与拆分使用同一动作，另有以下参数：

```json
{"operation":"merge","table":1,"row":1,"column":1,"endColumn":2,"expectedCell":"P3_A1"}
```

merge只接受同一行连续2–20个非空、未合并的单元格，endColumn是包含在选区内的终点列。写前还要求目标整表每格为明确 dxa 首选宽度，并与导出逻辑网格宽度相符（1 dxa 换算容差）；否则原生合并可能同步全表首选宽度，必须拒绝，不把该副作用当作等价变化。实际运行时布局仍以严格保存及刷新后验为准。split使用`columns`代替endColumn，且必须等于目标当前的横向gridSpan（2–20），只拆为一行，要求既有网格等宽。合并完整保留各格段落；原生合并会在移动段落的直接 run 内生成一个空 `w:rPr`，仅目标段落允许忽略无属性、无子节点、无文本且唯一的该节点，非目标和拆分比较仍保留原始 XML 严格校验。拆分将所有段落保留在第一格，其余新格为空，不推测原文分布。expectedCell须使用当时单元格的完整文本，多段落以`\n`连接。纵向合并和嵌套表格不在此路径支持范围。每次操作都需独立的中间导出证据；2×2专用测试文稿已完成逐步合并/拆分补充验证；不等宽 3999/4001 网格被写前拒绝，显式均分为 4000/4000 后另行合并/拆分通过，未放宽前置条件。

30.1 主验收使用另一个 3×3 私有文稿，基线含 A1–C3 九个唯一标记，依次完成插入行、插入列、删除原 A3/B3/C3 行、删除原 C1/C2 列、合并、拆分、均分行、均分列。每一步均有独立 DOCX 与刷新截图；最终行高均为 960 dxa、列宽均为 2000 dxa。合并的原命令因空 `w:rPr` 后验失败，保留 exit 1；经窄化回归与独立审查后，另行只读恢复 exit 0，未重放写入。此前固定总宽文稿触发全表首选宽度副作用的失败仍单列，不归为成功。完整八步与逐条命令哈希见 `tmp/phase3-live/docx-slides/30-table-compact/final-handoff-index.json`。

## document-table-style

```json
{"table":1,"row":1,"column":1,"expectedCell":"P3_A1","fill":"#0000FF","color":"#FFFFFF","marginCm":0.2,"align":"center","border":{"color":"#000000","pt":1}}
```

fill/color只改指定单格；marginCm、align、border作用整张目标表。若表头有多格，分别运行；其他格只传fill/color即可。marginCm>0且≤2厘米，四边相同。align=left/center/right，必须同时明确marginCm，避免SDK默认重置未指定内边距。border为整表外边框单实线，pt范围0.25–12磅且1/8步长。无属性请求与已经符合的请求均拒绝。请求文字色时 expectedCell 不得为空。所有单位已源代码查证，线上验收单独记录。

## document-graphic

```json
{"name":"PHASE3_ARROW","description":"PHASE3_ARROW_20261118","widthCm":6,"heightCm":4,"rotationDegrees":90,"wrap":"square","anchor":"paragraph","horizontalAlign":"center","horizontalRelativeTo":"margin"}
```

目标支持两种互斥方式：`name` 和 `description` 均精确匹配同一个真实图形；或明确给出 `paragraph`、`expectedParagraph`、`drawing`（段落序号和段内图形序号从 1 开始，完整段落文字可为空）。例如 `{"paragraph":2,"expectedParagraph":"","drawing":1,"rotationDegrees":90}`。位置目标仅支持纯段落正文，写入前核对整篇段落文字、各段图形数量及每个图形的类型、标准几何、尺寸与旋转；包含表格、结构不匹配或几何不明确时拒绝。它适用于导入后原生名称元数据缺失的图形，不能与名称目标混用。widthCm/heightCm必须同时给出，范围0.01–50厘米；rotationDegrees是绝对角度，范围[0,360)。wrap支持square/topAndBottom/behind/inFront。anchor选择现有承载段落或页面的垂直参照零点（paragraph/page），不移动到其他段落、不锁定锚点。水平对齐仅支持center，必须同时指定page或margin参照。嵌入图形转为浮动时必须明确wrap、anchor及水平对齐参照。

真实原生形状还支持`fill:"#0000FF"`和`line:{"color":"#FF0000","widthPt":2}`；线宽单位为磅，图片不能冒充原生形状。此动作核对实际原生选择、导出属性、非目标内容及重新打开后的结果。预设样式支持独立请求 `{"paragraph":2,"expectedParagraph":"","drawing":1,"preset":2}`：1–6 对应原生“形状样式”工具栏第一排的实际可见选项，预览文字均为 Abc。每次核对原生选项值、定义和可见颜色后点击该选项；保存、刷新和 DOCX 导出验证填充、线色与字体引用，保留未指定线宽及其他属性。预设不能与尺寸、旋转、填充、线条或布局合并；需要先保留预设的独立中间证据，再执行直接颜色覆盖。线上记录包含原始提交失败与后续独立只读恢复，完整状态见[验收记录](phase3-acceptance.md)，恢复不改写原失败结果。

## document-mention

```json
{"paragraph":1,"text":"PHASE3_SELF_MENTION_20261118"}
```

仅追加当前users/me本人真实用户提及，禁止传userId或姓名；目标段落已有用户提及则拒绝。刷新后检查真实节点atType=1、atInfo.userId、显示名、nodeId；导出核对追加文本、旧字符属性、包资源与非文本结构保持，仅去除本次追加本人姓名及其精确原生超链接外壳后比较。当前动作不调用独立通知API，不保证平台自身不会产生通知；不支持他人提及。线上验收单独记录。
