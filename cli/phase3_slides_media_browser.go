package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

func phase3PrepareSlideMediaBrowser(b *phase3EditorBrowser, s *phase3SlideMediaSpec, slideID string) error {
	raw, e := os.ReadFile(s.File)
	if e != nil {
		return e
	}
	if len(raw) != s.Size || fmt.Sprintf("%x", sha256.Sum256(raw)) != s.SHA256 {
		return fmt.Errorf("媒体在参数校验后发生变化")
	}
	payload, _ := json.Marshal(map[string]any{"spec": s, "data": base64.StdEncoding.EncodeToString(raw), "slideId": slideID})
	v, e := b.Eval(`(async()=>{const p=` + string(payload) + `,s=p.spec;const sdk=window.presentationSDK,wb=sdk?.editor?.workbench;if(wb?.isReady()!==true||wb.permissions?.canEdit?.()!==true||wb.media?.queryInsertVideo?.().enabled!==true||wb.command?.canExecCommand?.('insert.'+s.kind)!==true||wb.media?.checkCanUpload?.()!==true)return {ok:false,error:'当前账号/部署未授权插入该类媒体'};const slide=sdk.API.slides.getSlides().find(x=>x.id===p.slideId);if(!slide)return {ok:false,error:'目标页面缺失'};if(wb.core.getSlideElements(wb.editor,slide.getIndex()).some(x=>x.name===s.name))return {ok:false,error:'页面已有同名对象，禁止重复插入'};const bytes=Uint8Array.from(atob(p.data),c=>c.charCodeAt(0)),file=new File([bytes],s.name,{type:s.mime});const checked=await wb.media.checkSelectFile(s.kind,{files:[file]});if(checked?.length!==1||checked[0]!==file)return {ok:false,error:'原生上传校验未接受该媒体'};const player=document.createElement(s.kind),blobURL=URL.createObjectURL(file);let duration;try{player.preload='auto';player.muted=true;await new Promise((resolve,reject)=>{const timer=setTimeout(()=>reject(Error('本地媒体解码超时')),15000);player.onloadeddata=()=>{clearTimeout(timer);resolve()};player.onerror=()=>{clearTimeout(timer);reject(Error('本地媒体无法解码'))};player.src=blobURL});duration=player.duration;if(!Number.isFinite(duration)||duration<=0||player.readyState<2)throw Error('媒体时长/解码结果无效');const uploaded=await wb.options.override.uploadFile(file);if(!uploaded?.url)throw Error('媒体上传未返回真实URL');const canvas=document.createElement('canvas');canvas.width=s.kind==='video'?player.videoWidth:320;canvas.height=s.kind==='video'?player.videoHeight:100;if(canvas.width<1||canvas.height<1)throw Error('媒体预览尺寸无效');const ctx=canvas.getContext('2d');if(s.kind==='video')ctx.drawImage(player,0,0);else{ctx.fillStyle='#F4F5F7';ctx.fillRect(0,0,320,100);ctx.fillStyle='#32363F';ctx.font='20px sans-serif';ctx.fillText('♫ '+s.name,16,56,288)}const blob=await new Promise((resolve,reject)=>canvas.toBlob(v=>v?resolve(v):reject(Error('媒体海报生成失败')),'image/png'));const poster=await wb.options.override.uploadFile(new File([blob],s.name+'.png',{type:'image/png'}));if(!poster?.url)throw Error('媒体海报上传失败');const loader=wb.editor._$imageLoader$_;if(typeof loader?._$loadImages$_!=='function'||typeof loader?._$getImage$_!=='function')throw Error('缺少可预加载海报的原生接口');await loader._$loadImages$_([{url:poster.url,encryptUrl:poster.encryptUrl}]);if(loader._$getImage$_(poster.url)?._$Status$_!==1)throw Error('媒体海报未成功加载，未写入');window.__shimoPhase3SlideMedia={slideId:p.slideId,spec:s,uploaded,poster,duration,size:{width:s.kind==='video'?canvas.width*Math.min(480/canvas.width,270/canvas.height):320,height:s.kind==='video'?canvas.height*Math.min(480/canvas.width,270/canvas.height):100}};return {ok:true,duration,size:file.size,sha256:s.sha256}}finally{player.pause();player.removeAttribute('src');player.load();URL.revokeObjectURL(blobURL)}})()`)
	if e != nil {
		return e
	}
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("媒体准备结果无效")
	}
	duration, ok := m["duration"].(float64)
	if !ok || duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		return fmt.Errorf("媒体解码时长无效")
	}
	s.Duration = duration
	return nil
}
func phase3SlideMediaMutationJS(s phase3SlidesSpec, ids []string) string {
	raw, _ := json.Marshal(s)
	list, _ := json.Marshal(ids)
	return `(async()=>{const r=` + string(raw) + `,expected=` + string(list) + `;` + phase3PresenceGuardJS() + `
const sdk=window.presentationSDK,wb=sdk?.editor?.workbench,c=wb?.core,e=wb?.editor,staged=window.__shimoPhase3SlideMedia;if(wb?.isReady()!==true||wb.permissions?.canEdit?.()!==true||wb.media?.queryInsertVideo?.().enabled!==true||wb.command.canExecCommand('insert.'+r.media.kind)!==true)return {ok:false,error:'未确认媒体编辑授权'};if(JSON.stringify(sdk.API.slides.getSlides().map(s=>s.id))!==JSON.stringify(expected)||staged?.slideId!==r.slideId||staged.spec.sha256!==r.media.sha256)return {ok:false,error:'媒体准备结果或页面已变'};const slide=sdk.API.slides.getSlides().find(s=>s.id===r.slideId);if(!slide||c.getSlideElements(e,slide.getIndex()).some(x=>x.name===r.media.name))return {ok:false,error:'媒体目标页面/对象已变化'};if(e._$imageLoader$_._$getImage$_(staged.poster.url)?._$Status$_!==1)return {ok:false,error:'海报缓存不可用，拒绝异步网络后再写入'};sdk.API.slides.setCurrentSlideIndex(slide.id);sdk.API.slides.setSelectedSlides([slide.id]);const before=c.getSlideElements(e,slide.getIndex()).map(x=>x.id);const added=await c[r.media.kind==='audio'?'addAudio':'addVideo'](e,staged.poster.url,staged.uploaded,{slideRefId:slide.id,size:staged.size,name:r.media.name});if(added?.length!==1||!added[0].id||before.includes(added[0].id))return {ok:false,error:'未获得唯一新媒体对象，禁止重放'};const object=c.getSlideImageLikes(e,slide.getIndex()).find(x=>x.id===added[0].id);if(!object||!object[r.media.kind]?.url)return {ok:false,error:'新对象没有真实媒体引用，禁止重放'};if(r.media.kind==='audio'){` + phase3PresenceGuardJS() + `if(wb.permissions.canEdit()!==true)return {ok:false,error:'插入后权限已变，不能写播放参数；禁止重放'};const settings=wb.animation.getAudioSettings();if(settings.some(x=>x.shapeId===object.id))return {ok:false,error:'媒体已有播放参数，拒绝覆盖'};c.applySlideAudios(e,[...settings,{shapeId:object.id,acrossSlideNum:1,triggerType:r.media.triggerType,hideWhenShow:false,loopUntilStopped:false,volume:80}])}delete window.__shimoPhase3SlideMedia;return {ok:true,id:object.id,refId:object.refId,name:object.name,kind:r.media.kind};})()`
}
func phase3VerifyReloadedMedia(b *phase3EditorBrowser, s phase3SlidesSpec, dir string) (any, error) {
	raw, _ := json.Marshal(s)
	v, e := b.EvalUserGesture(`(async()=>{const r=` + string(raw) + `,s=r.media,sdk=window.presentationSDK,wb=sdk.editor.workbench,c=wb.core,e=wb.editor;const slide=sdk.API.slides.getSlides().find(x=>x.id===r.slideId);if(!slide)return {ok:false,error:'重开媒体页缺失'};sdk.API.slides.setCurrentSlideIndex(slide.id);const list=c.getSlideImageLikes(e,slide.getIndex()).filter(x=>x.name===s.name&&x[s.kind]?.url);if(list.length!==1)return {ok:false,error:'重开没有唯一真实媒体'};const object=list[0];let setting=null;if(s.kind==='audio'){const settings=wb.animation.getAudioSettings().filter(x=>x.shapeId===object.id);if(settings.length!==1)return {ok:false,error:'音频播放设置缺失'};setting=settings[0];if(setting.triggerType!==s.triggerType||setting.acrossSlideNum!==1||setting.hideWhenShow!==false||setting.loopUntilStopped!==false||setting.volume!==80)return {ok:false,error:'音频播放参数未持久化'}}if(wb.permissions.canCopy?.()!==true||wb.options.attachment?.[s.kind]?.download?.disabled===true)return {ok:false,error:'当前账号禁止下载媒体'};const url=wb.media.checkMediaUrl(object[s.kind].url);if(!url)return {ok:false,error:'媒体URL无效'};const response=await fetch(url);if(!response.ok)throw Error('媒体下载失败:'+response.status);const bytes=await response.arrayBuffer();if(bytes.byteLength!==s.size)throw Error('下载媒体字节数不同');const sha=[...new Uint8Array(await crypto.subtle.digest('SHA-256',bytes))].map(n=>n.toString(16).padStart(2,'0')).join('');if(sha!==s.sha256)throw Error('下载媒体SHA256不同');if(s.kind==='video')await wb.media.playVideo({url:object.video.url});else await wb.media.toggleAudio({url:object.audio.url,action:'start',setting,objectId:object.id});let found,first,last;const observed=[];for(let i=0;i<50;i++){await new Promise(r=>setTimeout(r,100));const elements=[...document.querySelectorAll(s.kind)];found=elements.find(n=>(` + phase3SlideMediaPlayerMatchesJS() + `)(n,url));if(!found)continue;observed.push({time:found.currentTime,duration:found.duration,readyState:found.readyState,paused:found.paused,error:found.error?.code||null});if(found.error)throw Error('原生播放器解码失败');if(found.readyState>=2){if(first===undefined)first=found.currentTime;last=found.currentTime;if(last-first>=0.15)break}}if(!found||last===undefined||last-first<0.15||!Number.isFinite(found.duration)||Math.abs(found.duration-s.decodedDuration)>Math.max(.25,s.decodedDuration*.02))throw Error('原生播放器没有可证实的解码/时间进度');if(!found.currentSrc||!(` + phase3SlideMediaPlayerMatchesJS() + `)(found,url))throw Error('播放资源与已校验媒体不一致');found.pause();let data='';const array=new Uint8Array(bytes);for(let i=0;i<array.length;i+=32768)data+=String.fromCharCode(...array.subarray(i,i+32768));return {ok:true,name:s.name,kind:s.kind,sha256:sha,bytes:bytes.byteLength,duration:found.duration,loadedSourceMatched:true,sourcePath:new URL(found.currentSrc).pathname,playback:observed,setting:setting?Object.fromEntries(['shapeId','acrossSlideNum','triggerType','hideWhenShow','loopUntilStopped','volume','duration'].filter(k=>setting[k]!==undefined).map(k=>[k,setting[k]])):null,data: btoa(data)}})()`)
	if e != nil {
		return nil, e
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("媒体重开验证无效")
	}
	encoded, ok := m["data"].(string)
	if !ok {
		return nil, fmt.Errorf("媒体下载未返回内容")
	}
	data, e := base64.StdEncoding.DecodeString(encoded)
	if e != nil {
		return nil, e
	}
	if len(data) != s.Media.Size || fmt.Sprintf("%x", sha256.Sum256(data)) != s.Media.SHA256 {
		return nil, fmt.Errorf("下载文件独立哈希校验失败")
	}
	folder := filepath.Join(dir, "downloaded-media")
	if e = os.MkdirAll(folder, 0700); e != nil {
		return nil, e
	}
	if e = os.WriteFile(filepath.Join(folder, s.Media.Name), data, 0600); e != nil {
		return nil, e
	}
	result := map[string]any{}
	for k, v := range m {
		if k != "data" {
			result[k] = v
		}
	}
	result["downloadPath"] = filepath.Join(folder, s.Media.Name)
	return result, nil
}

func phase3SlideMediaPlayerMatchesJS() string {
	return `(element,url)=>{const expected=new URL(url,location.href).href;return (element.currentSrc||element.src)===expected}`
}
