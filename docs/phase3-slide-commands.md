# 三期幻灯片命令参考

**链接先内置展示，操作按工具能力选择。** 本页列出外部 CLI 通道的固定动作；`slide-inspect`、`slide-design-inspect` 和页面编辑会自行启动临时本机浏览器，部分写后核验还会重开窗口，并不复用 WorkBuddy 内置标签页。内置浏览器能操作时优先使用；只有预览能力时可用外部 Computer Use 在原文件页面检查、编辑和核验，完成后再内置展示链接，不必先运行这些命令。`slide-print-pdf` 是导出动作，不能代替在内置页面查看背景、页数或保存状态。选择规则见[浏览器优先级](scenarios.md#浏览器优先级)。

当前实现接口与线上验收分开；完整场景状态见 [验收记录](phase3-acceptance.md)。

Use `phase3 <presentation-url> --action <action> [--payload '<JSON>'] --out <evidence-dir> --json`.
Obtain live IDs and exact shape/table names from `slide-design-inspect`; do not guess them.

- `slide-inspect`: no payload; reads page IDs.
- `slide-design-inspect`: no payload; reads page IDs/master/layout, available layouts/official themes, shape `{id,name,type,text}` and table `{id,name,rowCount,columnCount}` whitelists.
- `slide-add`, `slide-duplicate`, `slide-hide`, `slide-delete`: `{"slideId":"actual-page-id"}`. Add/duplicate insert immediately after anchor. Hide refuses already hidden. Delete refuses last page. Delete operates only supplied existing page.
- `slide-text-insert`: `{"slideId":"actual-page-id","text":"补充说明","x":120,"y":300,"width":650,"height":80}`. Pixel position/size; fontSize defaults24pt and bold defaultsfalse. Optional `fontSize`, `bold`. Single-line text only; bounds checked against PPTX page. Requires new shape ID and exact text/EMU geometry; no append-to-existing fallback.
- `slide-text-format`: `{"slideId":"actual-page-id","text":"exact unique full shape text","fontFamily":"Arial","fontSize":28,"color":"#0000FF"}`. At least one of fontFamily/fontSize/color/bold/italic/underline/outlineColor. Whole unique text shape; size pt; color is **text fill RGB**, not shape fill. `outlineColor` plus optional `outlineWidth` (points, defaults to 1) uses real run-level text outline; the target's existing explicit outline or an outline in its linked layout/master/theme/default text styles is conservatively rejected before such edits to preserve existing line styles. Shape borders cannot satisfy this requirement. False bold/italic/underline requires explicit exported false/none (unresolved inherited state fails proof).
- `slide-placeholder-fill`: `{"slideId":"actual-page-id","shapeName":"actual unique placeholder name","text":"三期验收"}`. Only empty title/ctrTitle/body/subTitle and text-bearing obj placeholders (including omitted type, whose default is obj). Use `\n` for multiple nonempty paragraphs. Nonempty existing text/empty paragraphs rejected. Preserves placeholder type/index/identity, other shape and body properties.
- `slide-theme`: `{"themeId":"actual-official-theme-id"}`. Applies one actually available official theme to the whole deck. Inspect supported IDs first. Currently supports the native default 16:9 page size (12192000 × 6858000 EMU) and requires every page's target layout to be uniquely matched by its native type/name. Rejects a target theme whose master already exists in the deck. Verifies the complete official master and all layout definitions, exact placeholder geometry, page/layout/theme relations, colors/fonts/images and fresh reopened state. Layout counts come from the actual official definitions. This implementation still needs live acceptance; it is separate from switching layouts within the current master.
- `slide-layout`: `{"slideId":"actual-page-id","layoutId":"actual-layout-id"}`. Only a layout already belonging to page's current master; cross-master/theme replacement is not implied. SDK layout indices are checked against exported master relations.
- `slide-shape-format`: `{"slideId":"actual-page-id","shapeName":"actual unique direct shape name","fillColor":"#0000FF","lineColor":"#FF0000","rotation":45,"horizontalCenter":true,"behindName":"actual unique reference object name"}`. At least one property; each property can be requested independently. Fill, line color and stacking do not require explicit geometry, so inherited-layout placeholders are supported. Rotation and horizontal centering still require a local transform; only horizontal centering reads the page width. fillColor/lineColor opaque RGB. lineColor requires existing explicit positive stroke width and preserves width; no line-width API exposed. rotation absolute degrees `[0,360)` converted internally from baseline to delta. horizontalCenter only true, relative to slide. behindName moves target immediately behind the named object, preserving other relative order. Grouped children/images unsupported as target. No arbitrary position input.
- `slide-table-edit` insert row: `{"slideId":"actual-page-id","tableName":"actual unique table name","operation":"insert-row","index":2}`. One-based index, inserts exactly one row **before** that existing row. Reject merged/irregular table.
- `slide-table-edit` delete column: `{"slideId":"actual-page-id","tableName":"actual unique table name","operation":"delete-column","index":3}`. One-based column; delete exactly one; refuse last column and merged/irregular table.
- `slide-table-edit` merge: `{"slideId":"actual-page-id","tableName":"actual unique table name","operation":"merge","row":1,"column":1}`. One-based top-left; exactly one horizontal pair (two adjacent columns, one row). No other merged cells. Target paragraphs must all contain non-whitespace text. No empty/whitespace-only paragraphs are silently dropped.
- `slide-table-edit` split: `{"slideId":"actual-page-id","tableName":"actual unique table name","operation":"split","row":1,"column":1}`. Only exact two-column horizontal merge; true core.splitCell(2,1), not SDK removeSpan (which currently merges). Combined text remains in left cell, right is empty; does not invent redistribution of original text. Each operation needs its own intermediate export.
- `slide-table-edit` style: `{"slideId":"actual-page-id","tableName":"actual unique table name","operation":"style","headerFillColor":"#0000FF","borderColor":"#000000","borderWidth":1}`. First row fill plus all table cell borders; either color optional but at least one required. Width0.25–20pt, defaults1pt when borderColor set; width without color rejected. Header fill alone supports absent/default cell style properties and preserves existing border styles; untouched body cells need no explicit style. Every target cell side and nonheader fill verified separately.
- `slide-replace-all`: `{"old":"旧版测试","new":"新版测试"}`. Entire deck including hidden pages; case-sensitive literal. No regex/newlines; old/new must differ; either may contain the other. Runs one literal replacement pass (no recursive replacement), verifies exact expected paragraph text and residual old/new counts. Refuses old text spanning formatted runs before write; verifies exact count/no other changes.
- `slide-title-animate`: `{"targets":[{"slideId":"page1-id","text":"exact full title1"},{"slideId":"page2-id","text":"exact full title2"}],"fontSize":24}`. One synchronous batch for1–100 unique pages, one exact unique shape per page. fontSize is optional: omit it to preserve all existing typography, or supply it to change only target font sizes; fixed real object entrance **Appear**, click trigger, duration1ms; not slide transition. Refuses existing page timing/animations to prevent overwrite. Exact timing target IDs and independent reopened animation data checked; actual preview remains an acceptance requirement. No custom animation params exposed.
- `slide-show-settings`: `{"loopShow":true,"useTiming":false}`. Either/both booleans required; each request needs only its own setter, with both settings read back to detect collateral changes. Native loop playback and use-slide-timings configuration. Checks ppt/presProps.xml and fresh editor readback. No page-range options. Does not hide pages as a substitute.
- `slide-media-insert`: `{"slideId":"actual-page-id","kind":"audio","file":"/absolute/path/test.wav","triggerType":"click"}`. `kind` is audio/video; input at most 8 MiB and must decode in the browser. Audio accepts click/auto trigger; video uses native playback controls and rejects an audio-only trigger. Refuses existing target-page timing. Checks actual persisted media bytes and downloaded SHA-256, fresh native properties and real playback progress. Live acceptance is still required.
- `slide-print-pdf`: no payload. Backend default exports all visible pages, one slide per PDF page. CLI returns `exported:true,verified:false,status:"exported-not-verified"`, sourcePages/expectedVisiblePages/hiddenSlideIndices; independent PDF parse/render/page-order acceptance required. Does not claim physical print, include-hidden or arbitrary ranges.

Every mutating action uses collaboration and permission guards, exports before/after, exact target proof and fresh reopen. Any failure after submission returns “可能已生效 / 禁止自动重放” and evidence path. The implementation rejects unknown properties; unsupported operations are not silently translated into similar-looking actions.

## 独立操作排查（2026-11-27）

单项请求只检查其实际依赖；未请求的文字、位置、边框、其他对象和页面仍需保持不变。以下为代码审查与回归测试范围，不代表各项均重新通过线上验收。

本次另有一次真实填充验收：对继承版式几何的 `Title 1` 单独设置 `fillColor="#0000FF"`，正式 CLI 返回 `ok:true, verified:true`；前后 PPTX 均无局部 transform，目标填充变为蓝色，其他内容差异检查及独立重开通过，重开截图已目检。其余新增分支为离线回归验证，不替代历史场景验收。

| 场景 | 命令 | 本次处理与保留边界 |
|---|---|---|
| 33 主题与版式 | slide-theme / slide-layout | 主题核验支持新模板移除空白未匹配标题框、保留未匹配内容及其原有效位置；已匹配对象仍按新官方模板核验。保留当前母版范围等验证。 |
| 33 页面管理 | slide-add / slide-duplicate / slide-hide / slide-delete | 未发现同类问题；保留页面顺序、末页删除保护。 |
| 34 文字格式 | slide-text-format | 文字轮廓检查限定到目标与其继承来源，无关页面/形状已有轮廓不再阻止操作；目标及相关继承来源已有轮廓仍拒绝覆盖。 |
| 34 插入与占位符 | slide-text-insert / slide-placeholder-fill | 支持带文本容器的普通内容占位符（obj 或省略 type）；保留空内容、对象身份和文本插入边界检查。 |
| 35 图形 | slide-shape-format | 填充、边框颜色、层级不再要求独立位置/大小；旋转、居中仍需可验证的显式几何，改边框色仍需已有线宽。 |
| 35 表格 | slide-table-edit | 只改表头填充不再要求正文单元格有显式样式；允许目标从默认样式新增填充，保留正文与原边框核验。 |
| 35 音视频 | slide-media-insert | 未确认同类错误；保留媒体权限、解码、字节及实际播放检查；没有新增独立媒体下载/任意参数设置入口。 |
| 36 动画 | slide-title-animate | fontSize 可省略；只加动画不再强制修改字号。保留目标页既有时序保护。 |
| 36 替换与放映 | slide-replace-all / slide-show-settings | 替换允许 old/new 互相包含，按一次替换后的完整段落核验；资源比较支持导出时图片重命名但保持字节与关系不变。单项放映设置只要求对应写接口；保留跨格式 run 保护。 |
| 36 打印 | slide-print-pdf | 保留导出与验收区分；不把 PDF 导出视作物理打印或自动完成版面验收。 |

例如，保留字号、只给一个标题加动画：

```sh
shimo-net phase3 '<演示稿链接>' --action slide-title-animate \
  --payload '{"targets":[{"slideId":"实际页面ID","text":"完整标题文字"}]}' --json
```

### 主题与替换失败样本补充（2026-11-27）

- `slide-theme`：原空白且无独立样式的未匹配模板框可由官方空白占位框替代；含文字、链接、图片或自定义样式的原对象不能因此丢失。未匹配的非空副标题保留原内容，若位置从继承变为显式，必须等于旧版式/母版的有效位置；已匹配的对象不享受这个例外。
- 主题重开核验只对 `spPr.extra.effects` 中外阴影的 `blurRad` 接受原始值或编辑器确切的 EMU→毫米→EMU 截断结果，不采用通用 ±1 容差，其他阴影属性仍精确比较。
- `slide-replace-all`：主题图片的导出文件名可重新生成。独立关系文件按关系 ID、类型、元数据及实际引用内容核验，不把随机文件名变化当成图片内容被修改；不同字节、缺失图片、关系改指、外链变化仍失败。
- `old="你好"`、`new="你好世界"` 等扩展替换，以及反向缩短替换均允许；`old == new` 仍拒绝。旧词可以按预期留在结果中，不再要求替换后旧词次数一律为零。仅替换已有内容一次，不重复替换刚生成的新词。
- 已尝试写入的失败命令仍禁止自动重放。原失败记录不改写；保存物证的复核、重新打开的持久化检查分别记录为只读验证。
