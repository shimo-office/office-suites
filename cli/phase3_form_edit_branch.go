package main

// React may retain an old DOM-attached fiber. Resolve HostRoot.current first,
// then locate this exact DOM node in the mounted tree. Never choose an alternate
// because its props happen to match the requested option.
const phase3CurrentPreviewFormJS = `((element,spec)=>{
 let anchor=element,fiber=null;
 for(let count=0;anchor&&count<100;count++,anchor=anchor.parentElement){const key=Object.keys(anchor).find(k=>k.startsWith('__reactFiber$'));if(key){fiber=anchor[key];break}}
 if(!fiber)return {error:'preview DOM has no React binding'};
 let root=fiber;const parents=new Set();
 while(root.return){if(parents.has(root)||parents.size>=200)return {error:'invalid preview parent chain'};parents.add(root);root=root.return}
 const current=root.stateNode?.current;
 if(root.tag!==3||current?.tag!==3||current.stateNode!==root.stateNode)return {error:'preview HostRoot.current unavailable'};
 const stack=[{node:current,parents:[]}],visited=new Set(),matches=[];
 while(stack.length){const item=stack.pop(),node=item.node;if(!node)continue;if(visited.has(node)||visited.size>=20000)return {error:'preview current tree is invalid or too large'};visited.add(node);const path=item.parents.concat(node);if(node.stateNode===anchor)matches.push(path);if(node.sibling)stack.push({node:node.sibling,parents:item.parents});if(node.child)stack.push({node:node.child,parents:path})}
 if(matches.length!==1)return {error:'preview DOM not uniquely mounted in current tree'};
 const path=matches[0];let props=null;
 for(let i=path.length-1;i>=0;i--){const candidate=path[i].memoizedProps;if(candidate?.mode==='preview'&&candidate.form){props=candidate;break}}
 if(!props||typeof props.onResponseChange!=='function')return {error:'current preview form ancestor unavailable'};
 const form=props.form,ids=Array.isArray(form.questions)?form.questions.map(q=>q.guid):[];
 if(form.title!==spec.title||ids.length!==spec.known.length||new Set(ids).size!==ids.length||JSON.stringify([...ids].sort())!==JSON.stringify([...spec.known].sort()))return {error:'current preview form identity mismatch'};
 return {form};
})`

// One synchronous observation is reused by readiness polling and the final
// evidence capture, so the final capture cannot inherit an earlier success.
const phase3FormBranchObservationJS = `(spec=>{
 const c=spec.branch,known=spec.known,input=document.getElementById(spec.trigger+'_'+c.option);
 const shown=id=>{const e=document.getElementById(id);return !!e&&[...e.getClientRects()].some(r=>r.width>0&&r.height>0)&&getComputedStyle(e).visibility!=='hidden'};
 const sequence=[...document.querySelectorAll('[id]')].filter(e=>known.includes(e.id)&&shown(e.id)).map(e=>e.id);
 const q=window.logged?.['fill:query'];let form,selectionError;
 if(spec.respondent){form=q?.getForm();const ids=form?.questions?.map(x=>x.guid);if(q?.getGuid?.()!==spec.guid||form?.title!==spec.title||!Array.isArray(ids)||ids.length!==known.length||new Set(ids).size!==ids.length||JSON.stringify([...ids].sort())!==JSON.stringify([...known].sort())){selectionError='respondent form identity mismatch';form=undefined}}
 else{const selected=(` + phase3CurrentPreviewFormJS + `)(input,spec);form=selected.form;selectionError=selected.error}
 const nativeSequence=spec.respondent?q?.getVisibleQuestions?.().map(x=>x.guid).filter(id=>known.includes(id)):undefined;
 const selectedOption=form?.questions?.find(x=>x.guid===spec.trigger)?.response?.choice?.value;
 const checks={input:!!input&&!input.disabled,selection:typeof selectedOption==='string'&&selectedOption===c.option,visible:c.visible.every(shown),hidden:c.hidden.every(id=>!shown(id)),sequence:!c.sequence||JSON.stringify(c.sequence)===JSON.stringify(sequence),native:!spec.respondent||(Array.isArray(nativeSequence)&&(!c.sequence||JSON.stringify(c.sequence)===JSON.stringify(nativeSequence)))};
 return {verified:Object.values(checks).every(Boolean),checks,selectionError,selectionSource:spec.respondent?'native-fill-query':'current-react-preview-form',selectedOption,selectionVerified:checks.selection,questionSequence:sequence,nativeQuestionSequence:nativeSequence,option:c.option,clickedOption:c.option,inputChecked:input?.checked,title:document.title,visible:c.visible.filter(shown),hidden:c.hidden.filter(id=>!shown(id)),questionRects:Object.fromEntries(known.map(id=>[id,document.getElementById(id)?[...document.getElementById(id).getClientRects()].map(r=>({width:r.width,height:r.height})):[]]))};
})`
