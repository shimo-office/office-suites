package main

func documentTableDistributeJS(s documentTableSpec, before documentTablesSnapshot) string {
	preflight := `if(typeof a.startAction!=='function'||typeof a.finalizeAction!=='function'||typeof table.getRowsLen!=='function'||typeof table.getRow!=='function')return {ok:false,error:'均分表格API未就绪'};let distribution;`
	mutation := `if(a.getCurTable()!==table)return {ok:false,error:'当前表格身份不符，未写入'};`
	if s.Operation == "distribute-rows" {
		preflight += `if(typeof table.getLayoutRowContentHeight!=='function'||table.getRowsLen()<2||table.children.some(r=>typeof r.setRowHeight!=='function'))return {ok:false,error:'行高模型不支持均分'};let total=0;for(let i=0;i<table.getRowsLen();i++)total+=table.getLayoutRowContentHeight(i);distribution=total/table.getRowsLen();if(!Number.isFinite(distribution)||distribution<=0)return {ok:false,error:'行高布局尚未计算'};`
		mutation += `a.startAction();try{for(let i=0;i<table.getRowsLen();i++)table.getRow(i).setRowHeight(distribution,0)}finally{a.finalizeAction()}`
	} else {
		preflight += `if(typeof table.getLayoutTable!=='function'||typeof table.getPreferredMeasurementType!=='function'||typeof table.rebuildFromPositions!=='function')return {ok:false,error:'列网格模型不支持均分'};const layout=table.getLayoutTable(),grid=layout?.getGridCol?.(),min=layout?.getGridColMin?.();if(!Array.isArray(grid)||grid.length<2||!Array.isArray(min))return {ok:false,error:'列宽布局未计算'};const width=Math.round(grid.reduce((a,b)=>a+b,0)/grid.length);if(!Number.isFinite(width)||width<=0||min.some(m=>m>width))return {ok:false,error:'均分宽度低于内容最小宽度，不改变布局模式'};distribution={width,type:table.getPreferredMeasurementType()[0]};if(distribution.type===undefined)return {ok:false,error:'列宽度量类型未知'};`
		mutation += `const positions=table.children.map(row=>Array.from({length:row.children.length+1},(_,i)=>i*distribution.width));a.startAction();try{table.rebuildFromPositions(positions,distribution.type)}finally{a.finalizeAction()}`
	}
	return documentTableOperationJS(s, before, "getCurTable", preflight, mutation)
}
