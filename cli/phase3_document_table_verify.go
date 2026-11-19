package main

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

type documentTableSpec struct {
	Operation    string `json:"operation"`
	Table        int    `json:"table"`
	Row          int    `json:"row"`
	Column       int    `json:"column"`
	ExpectedCell string `json:"expectedCell"`
	EndColumn    int    `json:"endColumn,omitempty"`
	Columns      int    `json:"columns,omitempty"`
}
type documentTableCell struct {
	RebuiltStyle       string   `json:"rebuiltStyle,omitempty"`
	Envelope           string   `json:"envelope,omitempty"`
	GridSpan           int      `json:"gridSpan,omitempty"`
	StartGridCol       int      `json:"startGridCol,omitempty"`
	VMerge             string   `json:"vMerge,omitempty"`
	Paragraphs         []int    `json:"paragraphs,omitempty"`
	ParagraphTexts     []string `json:"paragraphTexts,omitempty"`
	ParagraphKeys      []string `json:"paragraphKeys,omitempty"`
	MergeParagraphKeys []string `json:"mergeParagraphKeys,omitempty"`
	CellProperties     string   `json:"cellProperties,omitempty"`
	WidthType          string   `json:"widthType,omitempty"`
	WidthValue         float64  `json:"widthValue,omitempty"`
	EmptyContent       bool     `json:"emptyContent,omitempty"`

	WidthProperties string `json:"widthProperties,omitempty"`
	Text            string `json:"text"`
	Paragraph       int    `json:"paragraph"`
	Style           string `json:"style"`
}
type documentTableHeight struct {
	Value float64 `json:"value"`
	Rule  string  `json:"rule"`
}
type documentTable struct {
	RebuiltRowStyles      []string              `json:"rebuiltRowProperties,omitempty"`
	RebuiltRowOtherStyles []string              `json:"rebuiltOtherRowProperties,omitempty"`
	RebuiltRowEnvelopes   []string              `json:"rebuiltRowEnvelopes,omitempty"`
	Envelope              string                `json:"envelope,omitempty"`
	RowEnvelopes          []string              `json:"rowEnvelopes,omitempty"`
	GridWidths            []float64             `json:"gridWidths"`
	RowHeights            []documentTableHeight `json:"rowHeights"`
	RowOtherStyles        []string              `json:"otherRowProperties"`
	Grid                  string                `json:"grid"`
	RawStructure          string                `json:"rawStructure"`
	Properties            string                `json:"properties"`
	Rows                  [][]documentTableCell `json:"rows"`
	RowStyles             []string              `json:"rowProperties,omitempty"`
}
type documentTablesSnapshot struct {
	Assets         map[string]string `json:"assets,omitempty"`
	StyleResources map[string]string `json:"styleResources,omitempty"`
	Tables         []documentTable   `json:"tables"`
	OutsideTables  string            `json:"outsideTables"`
}

var documentTableMethods = map[string]string{"insert-row-before": "addTblRowAbove", "insert-row-after": "addTblRowBelow", "delete-row": "removeRow", "insert-column-before": "addTblColumnLeft", "insert-column-after": "addTblColumnRight", "delete-column": "removeColumn", "distribute-rows": "getCurTable", "distribute-columns": "getCurTable", "merge": "mergeCells", "split": "splitCell"}

func prepareDocumentTable(p map[string]any) (documentTableSpec, error) {
	s := documentTableSpec{}
	for k := range p {
		if k != "operation" && k != "table" && k != "row" && k != "column" && k != "expectedCell" && k != "endColumn" && k != "columns" {
			return s, fmt.Errorf("document-table 未知参数 %s", k)
		}
	}
	s.Operation, _ = p["operation"].(string)
	if documentTableMethods[s.Operation] == "" {
		return s, fmt.Errorf("document-table 只支持已定义的行列插删、均分、横向合并拆分动作")
	}
	for key, dst := range map[string]*int{"table": &s.Table, "row": &s.Row, "column": &s.Column} {
		n, ok := p[key].(float64)
		if !ok || n < 1 || n > 100000 || math.Trunc(n) != n {
			return s, fmt.Errorf("%s 必须为从1开始的整数", key)
		}
		*dst = int(n)
	}
	text, ok := p["expectedCell"].(string)
	if !ok || utf8.RuneCountInString(text) > 100000 {
		return s, fmt.Errorf("expectedCell 必须精确指定定位单元格文本")
	}
	s.ExpectedCell = text
	if err := prepareDocumentTableSpanParams(p, &s); err != nil {
		return s, err
	}
	return s, nil
}

func documentTableNodeKey(n *artifactNode, stripTables bool) string {
	if n == nil {
		return ""
	}
	var copyNode func(*artifactNode) *artifactNode
	copyNode = func(x *artifactNode) *artifactNode {
		clone := *x
		clone.Children = nil
		if stripTables && x.Name.Space == artifactWordNS && x.Name.Local == "tbl" {
			clone.Attr = nil
			clone.Text = ""
			return &clone
		}
		for _, c := range x.Children {
			if !stripTables && c.Name.Space == artifactWordNS && c.Name.Local == "tcW" {
				continue
			}
			clone.Children = append(clone.Children, copyNode(c))
		}
		return &clone
	}
	return documentTableExactKey(copyNode(n))
}

// Keep text whitespace exact: leading/trailing spaces are meaningful document content.
func documentTableExactKey(n *artifactNode) string {
	if n == nil {
		return ""
	}
	attrs := make([]string, 0, len(n.Attr))
	for _, a := range n.Attr {
		attrs = append(attrs, a.Name.Space+"|"+a.Name.Local+"="+a.Value)
	}
	sort.Strings(attrs)
	children := make([]string, 0, len(n.Children))
	for _, c := range n.Children {
		children = append(children, documentTableExactKey(c))
	}
	text := n.Text
	if len(n.Children) > 0 && strings.TrimSpace(text) == "" {
		text = ""
	}
	data, _ := json.Marshal([]any{n.Name.Space, n.Name.Local, attrs, text, children})
	return string(data)
}

func readDocumentTables(filename string) (documentTablesSnapshot, error) {
	var result documentTablesSnapshot
	z, entries, err := openArtifact(filename)
	if err != nil {
		return result, err
	}
	defer z.Close()
	graphic, resourceErr := readDocumentGraphic(filename)
	if resourceErr != nil {
		return result, resourceErr
	}
	result.Assets = graphic.Assets
	result.StyleResources = graphic.Resources
	rebuildDefaultsSafe := true
	for resource := range result.StyleResources {
		if !strings.HasPrefix(resource, "part:") {
			continue
		}
		data, e := zipEntryBytes(entries[strings.TrimPrefix(resource, "part:")])
		if e != nil {
			return result, e
		}
		styles, e := parseArtifactXML(data)
		if e != nil {
			return result, e
		}
		for _, name := range []string{"gridBefore", "gridAfter", "wBefore", "wAfter", "gridSpan"} {
			if len(documentGraphicNodes(styles, artifactWordNS, name)) > 0 {
				rebuildDefaultsSafe = false
			}
		}
	}
	doc, err := artifactPart(entries, "word/document.xml", artifactWordNS, "document")
	if err != nil {
		return result, err
	}
	body := doc.child("body")
	if body == nil {
		return result, fmt.Errorf("文稿正文缺失")
	}
	result.OutsideTables = documentTableNodeKey(body, true)
	pIndex := map[*artifactNode]int{}
	for i, p := range documentChildren(body, "p") {
		pIndex[p] = i + 1
	}
	for _, table := range documentChildren(body, "tbl") {
		out := documentTable{Envelope: documentSpanEnvelope(table, "tr"), Properties: documentTableExactKey(table.child("tblPr")), Grid: documentTableExactKey(table.child("tblGrid")), RawStructure: documentTableExactKey(table)}
		if grid := table.child("tblGrid"); grid != nil {
			for _, col := range grid.children("gridCol") {
				w, _ := strconv.ParseFloat(documentAttr(col, "w"), 64)
				out.GridWidths = append(out.GridWidths, w)
			}
		}
		for _, row := range table.children("tr") {
			out.RowEnvelopes = append(out.RowEnvelopes, documentSpanEnvelope(row, "tc"))
			rebuilt := documentStyleClone(row)
			rebuiltOther := documentStyleClone(row)
			documentStyleStrip(rebuiltOther.child("trPr"), "trHeight")
			if rebuildDefaultsSafe {
				rebuilt = documentTableRebuiltRow(row, false)
				rebuiltOther = documentTableRebuiltRow(row, true)
			}
			out.RebuiltRowStyles = append(out.RebuiltRowStyles, documentTableExactKey(rebuilt.child("trPr")))
			out.RebuiltRowOtherStyles = append(out.RebuiltRowOtherStyles, documentTableExactKey(rebuiltOther.child("trPr")))
			out.RebuiltRowEnvelopes = append(out.RebuiltRowEnvelopes, documentSpanEnvelope(rebuilt, "tc"))
			rp := documentStyleClone(row.child("trPr"))
			height := documentStyleChild(rp, "trHeight")
			value, _ := strconv.ParseFloat(documentAttr(height, "val"), 64)
			out.RowHeights = append(out.RowHeights, documentTableHeight{Value: value, Rule: documentAttr(height, "hRule")})
			documentStyleStrip(rp, "trHeight")
			if rp != nil && len(rp.Children) == 0 && len(rp.Attr) == 0 {
				rp = nil
			}
			out.RowOtherStyles = append(out.RowOtherStyles, documentTableExactKey(rp))
			out.RowStyles = append(out.RowStyles, documentNodeKey(row.child("trPr")))
			cells := []documentTableCell{}
			for _, cell := range row.children("tc") {
				if len(documentChildren(cell, "tbl")) > 0 {
					return result, fmt.Errorf("嵌套表格尚无可靠结构定位，不执行写入")
				}
				parsed, e := readDocumentSpanCell(cell, pIndex)
				if e != nil {
					return result, e
				}
				if !rebuildDefaultsSafe {
					parsed.RebuiltStyle = parsed.Style
				}
				for _, previous := range cells {
					parsed.StartGridCol += documentCellSpan(previous)
				}
				cells = append(cells, parsed)
			}
			if len(cells) == 0 {
				return result, fmt.Errorf("空表格行不可定位")
			}
			if len(out.Rows) > 0 && documentRowGridWidth(cells) != documentRowGridWidth(out.Rows[0]) {
				return result, fmt.Errorf("表格各行逻辑grid跨度不一致")
			}
			if len(out.GridWidths) > 0 && documentRowGridWidth(cells) != len(out.GridWidths) {
				return result, fmt.Errorf("表格单元格跨度与tblGrid列数不一致")
			}
			out.Rows = append(out.Rows, cells)
		}
		if len(out.Rows) == 0 {
			return result, fmt.Errorf("空表格不可定位")
		}
		result.Tables = append(result.Tables, out)
	}
	return result, nil
}

func documentTableAnchor(before documentTablesSnapshot, s documentTableSpec) (documentFormatSpec, error) {
	if s.Table < 1 || s.Table > len(before.Tables) {
		return documentFormatSpec{}, fmt.Errorf("目标表格不存在")
	}
	t := before.Tables[s.Table-1]
	if s.Row < 1 || s.Row > len(t.Rows) || s.Column < 1 || s.Column > len(t.Rows[s.Row-1]) {
		return documentFormatSpec{}, fmt.Errorf("目标行列不存在")
	}
	cell := t.Rows[s.Row-1][s.Column-1]
	if cell.Text != s.ExpectedCell {
		return documentFormatSpec{}, fmt.Errorf("定位单元格文本不符 expectedCell")
	}
	if s.Operation == "delete-row" && len(t.Rows) == 1 || s.Operation == "delete-column" && len(t.Rows[0]) == 1 {
		return documentFormatSpec{}, fmt.Errorf("禁止用行列命令隐式删除整个表格")
	}
	if s.Operation == "merge" || s.Operation == "split" {
		if err := validateDocumentTableSpan(before, s); err != nil {
			return documentFormatSpec{}, err
		}
		text := cell.ParagraphTexts[0]
		return documentFormatSpec{Paragraph: cell.Paragraph, Text: text, End: utf8.RuneCountInString(text)}, nil
	}
	distribution := s.Operation == "distribute-rows" || s.Operation == "distribute-columns"
	for _, table := range before.Tables {
		for _, row := range table.Rows {
			for _, c := range row {
				// A restart without any continuation is a one-cell group. The
				// whole-table scan still rejects every continuation; native selection
				// additionally requires getVMergedRows() == 1 before any write.
				if documentCellSpan(c) != 1 || (c.VMerge != "" && c.VMerge != "restart") || (!distribution && len(c.Paragraphs) > 1) {
					return documentFormatSpec{}, fmt.Errorf("此操作不处理合并或多段落单元格")
				}
				if distribution && (len(c.Paragraphs) == 0 || len(c.Paragraphs) != len(c.ParagraphTexts) || c.Text != strings.Join(c.ParagraphTexts, "\n")) {
					return documentFormatSpec{}, fmt.Errorf("均分所需的完整单元格段落不可验证")
				}
			}
		}
	}
	text := cell.Text
	if distribution {
		text = cell.ParagraphTexts[0]
	}
	return documentFormatSpec{Paragraph: cell.Paragraph, Text: text, End: utf8.RuneCountInString(text)}, nil
}

func verifyDocumentTable(before, after documentTablesSnapshot, s documentTableSpec) error {
	if s.Operation == "merge" || s.Operation == "split" {
		return verifyDocumentTableSpan(before, after, s)
	}
	if !reflect.DeepEqual(before.Assets, after.Assets) {
		return fmt.Errorf("表格操作改变了非目标资源内容或关系绑定")
	}
	if err := verifyDocumentResources(before.StyleResources, after.StyleResources); err != nil {
		return err
	}
	if _, err := documentTableAnchor(before, s); err != nil {
		return err
	}
	// Newly inserted blank cells are not compared to an old cell below, but
	// must not turn a one-cell restart into a real vertical merge.
	for _, table := range after.Tables {
		for _, row := range table.Rows {
			for _, cell := range row {
				if documentCellSpan(cell) != 1 || (cell.VMerge != "" && cell.VMerge != "restart") {
					return fmt.Errorf("行列操作产生了未请求的合并结构")
				}
			}
		}
	}
	if len(before.Tables) != len(after.Tables) || before.OutsideTables != after.OutsideTables {
		return fmt.Errorf("目标表格以外的文稿结构改变")
	}
	for ti, bt := range before.Tables {
		at := after.Tables[ti]
		if bt.Properties != at.Properties {
			return fmt.Errorf("第%d表属性意外改变", ti+1)
		}
		target := ti == s.Table-1
		if !target && bt.RawStructure != at.RawStructure {
			return fmt.Errorf("第%d个非目标表格结构改变", ti+1)
		}
		rowDelta, colDelta := 0, 0
		rowAt, colAt := s.Row-1, s.Column-1
		if target {
			switch s.Operation {
			case "insert-row-before":
				rowDelta = 1
			case "insert-row-after":
				rowDelta = 1
				rowAt++
			case "delete-row":
				rowDelta = -1
			case "insert-column-before":
				colDelta = 1
			case "insert-column-after":
				colDelta = 1
				colAt++
			case "delete-column":
				colDelta = -1
			}
		}
		if target && (s.Operation == "distribute-rows" || s.Operation == "distribute-columns") {
			if err := verifyDocumentDistribution(bt, at, s.Operation); err != nil {
				return err
			}
		}
		distributeRows := target && s.Operation == "distribute-rows"
		distributeColumns := target && s.Operation == "distribute-columns"
		rebuildColumns := distributeColumns || (target && colDelta != 0)
		if colDelta == 0 && !distributeColumns && bt.Grid != at.Grid {
			return fmt.Errorf("未请求列变化但列网格宽度改变")
		}
		if len(at.Rows) != len(bt.Rows)+rowDelta {
			return fmt.Errorf("第%d表行数不符请求", ti+1)
		}
		for ar, acells := range at.Rows {
			if len(acells) != len(bt.Rows[0])+colDelta {
				return fmt.Errorf("第%d表列数不符请求", ti+1)
			}
			br := ar
			if rowDelta == 1 {
				if ar == rowAt {
					for _, c := range acells {
						if c.Text != "" {
							return fmt.Errorf("新行应为空白")
						}
					}
					continue
				}
				if ar > rowAt {
					br--
				}
			} else if rowDelta == -1 && ar >= rowAt {
				br++
			}
			if distributeRows || rebuildColumns {
				bt.RowStyles, bt.RowOtherStyles, _ = documentTableRebuiltKeys(bt)
				at.RowStyles, at.RowOtherStyles, _ = documentTableRebuiltKeys(at)
			}
			if distributeRows {
				if len(bt.RowOtherStyles) != len(at.RowOtherStyles) || len(bt.RowOtherStyles) <= br || bt.RowOtherStyles[br] != at.RowOtherStyles[ar] {
					return fmt.Errorf("均分行高改变了其他行属性")
				}
			}
			if !distributeRows && len(bt.RowStyles) > br && len(at.RowStyles) > ar && bt.RowStyles[br] != at.RowStyles[ar] {
				return fmt.Errorf("原有行属性意外改变")
			}
			for ac, c := range acells {
				bc := ac
				if colDelta == 1 {
					if ac == colAt {
						if c.Text != "" {
							return fmt.Errorf("新列应为空白")
						}
						continue
					}
					if ac > colAt {
						bc--
					}
				} else if colDelta == -1 && ac >= colAt {
					bc++
				}
				old := bt.Rows[br][bc]
				if colDelta == 0 && !distributeColumns && old.WidthProperties != c.WidthProperties {
					return fmt.Errorf("未请求列变化但单元格宽度改变")
				}
				oldStyle, newStyle := old.Style, c.Style
				if rebuildColumns && old.RebuiltStyle != "" && c.RebuiltStyle != "" {
					oldStyle, newStyle = old.RebuiltStyle, c.RebuiltStyle
				}
				if c.Text != old.Text || newStyle != oldStyle {
					return fmt.Errorf("原单元格%d:%d正文或样式改变", br+1, bc+1)
				}
			}
		}
	}
	return nil
}

func verifyDocumentDistribution(before, after documentTable, operation string) error {
	if before.RawStructure == after.RawStructure {
		return fmt.Errorf("均分请求没有产生可验证结构变化")
	}
	if operation == "distribute-rows" {
		if len(after.RowHeights) != len(after.Rows) || len(after.Rows) < 2 {
			return fmt.Errorf("没有可均分的多行高度")
		}
		height := after.RowHeights[0].Value
		for _, h := range after.RowHeights {
			if height <= 0 || h.Rule != "atLeast" || math.Abs(h.Value-height) > 1 {
				return fmt.Errorf("行高未持久化为一致的最小高度")
			}
		}
		return nil
	}
	if len(after.Rows) == 0 || len(after.GridWidths) != len(after.Rows[0]) || len(after.GridWidths) < 2 {
		return fmt.Errorf("列宽网格不可验证")
	}
	width := after.GridWidths[0]
	sumBefore, sumAfter := 0.0, 0.0
	for _, v := range before.GridWidths {
		sumBefore += v
	}
	for _, v := range after.GridWidths {
		if width <= 0 || math.Abs(v-width) > 1 {
			return fmt.Errorf("网格列宽未均分")
		}
		sumAfter += v
	}
	if math.Abs(sumBefore-sumAfter) > float64(len(after.GridWidths)) {
		return fmt.Errorf("均分列宽改变表格总宽")
	}
	return nil
}
