package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

type phase3SlideThemeSpec struct {
	Geometry     []map[string]phase3ThemeTransform `json:"expectedPlaceholderGeometry,omitempty"`
	Master       any                               `json:"expectedMaster,omitempty"`
	Layouts      []any                             `json:"expectedLayouts,omitempty"`
	SlideLayouts map[string]int                    `json:"expectedSlideLayoutIndexes,omitempty"`
	ImageKeys    map[string]string                 `json:"imageKeys,omitempty"`
	ID           string                            `json:"themeId"`
	Source       map[string]string                 `json:"officialSource,omitempty"`
	Expected     map[string]any                    `json:"expectedNativeTheme,omitempty"`
	LayoutCount  int                               `json:"layoutCount,omitempty"`
	Images       []string                          `json:"imageSHA256,omitempty"`
}

func preparePhase3SlideTheme(p map[string]any) (*phase3SlideThemeSpec, error) {
	if len(p) != 1 {
		return nil, fmt.Errorf("slide-theme 仅接受官方 themeId，作用于全部页面")
	}
	id, e := phase3LayoutID(p["themeId"])
	if e != nil {
		return nil, e
	}
	return &phase3SlideThemeSpec{ID: id}, nil
}

// Stage official source and images before the last presence gate. Uploading an
// asset does not mutate the presentation. Signed upload URLs stay in the page.
func phase3PrepareSlideTheme(b *phase3EditorBrowser, s *phase3SlideThemeSpec) error {
	id, _ := json.Marshal(s.ID)
	v, e := b.Eval(`(async()=>{const id=` + string(id) + `;const sdk=window.presentationSDK,wb=sdk?.editor?.workbench;if(wb?.isReady()!==true||wb.permissions?.canEdit?.()!==true)return {ok:false,error:'未确认编辑权限'};const theme=wb.theme?.getOfficialThemes?.().find(t=>t.id===id);if(!theme||typeof theme.theme!=='function'||!Number.isInteger(theme.layoutCount)||theme.layoutCount<1||theme.layoutCount>100)return {ok:false,error:'必须使用当前官方主题列表中唯一的themeId'};if(wb.editor.presentation.slideMasters.some(m=>m.themeId===id))return {ok:false,error:'目标主题母版已存在，不能复用可能已被修改的主题'};if(typeof wb.options?.override?.uploadFile!=='function'||typeof wb.core?.applyOfficalTheme!=='function')return {ok:false,error:'主题上传/应用接口缺失'};const definition=(await theme.theme()).default;if(!definition||['masters','layouts','themes'].some(k=>typeof definition[k]!=='string'||definition[k].length<1||definition[k].length>2000000))return {ok:false,error:'官方主题定义无效'};const Delta=window.modoc?.Delta;if(typeof Delta!=='function'||typeof wb.editor._$collaborationManager$_?._$currentPptIOData$_!=='function')return {ok:false,error:'缺少完整主题Delta只读接口'};if(Math.abs(wb.editor.presentation.width-3048/9)>1e-7||Math.abs(wb.editor.presentation.height-190.5)>1e-7)return {ok:false,error:'当前主题严格验证仅支持原生16:9默认页面尺寸，未写入'};const decoded=Object.fromEntries(['masters','layouts','themes'].map(k=>[k,new Delta(definition[k]).toJSON()]));const rootOps=wb.editor._$ioAdapter$_.fromOperations(wb.editor._$collaborationManager$_._$currentPptIOData$_()).toJSON();const layoutMap=rootOps.find(x=>x.attributes?.opType==='layouts')?.data?.slide;if(!layoutMap)return {ok:false,error:'当前持久化版式操作缺失'};const currentLayouts=sdk.API.slides.getSlides().map((s,i)=>({id:s.id,definition:layoutMap[wb.editor.presentation.slides[i].layout.refId]}));const images={},hashes=[],imageKeys={};for(const [key,value]of Object.entries(theme.images||{})){const url=new URL(value,location.href);if(!['https:','http:'].includes(url.protocol))return {ok:false,error:'主题资源URL不受支持'};const response=await fetch(url.href);if(!response.ok)throw Error('主题资源下载失败:'+response.status);const bytes=await response.arrayBuffer();if(!bytes.byteLength||bytes.byteLength>20000000)throw Error('主题资源大小无效');const hash=[...new Uint8Array(await crypto.subtle.digest('SHA-256',bytes))].map(n=>n.toString(16).padStart(2,'0')).join('');hashes.push(hash);imageKeys[key]=hash;const name=url.pathname.split('/').pop();const mime=response.headers.get('content-type')?.split(';')[0]||(/\.jpe?g$/i.test(name)?'image/jpeg':'image/png');const uploaded=await wb.options.override.uploadFile(new File([bytes],name,{type:mime}));if(!uploaded||typeof uploaded.url!=='string'||!uploaded.url)throw Error('主题资源上传未返回URL');images[key]=uploaded.url}window.__shimoPhase3SlideTheme={id,definition,images};return {ok:true,definition,decoded,currentLayouts,layoutCount:theme.layoutCount,imageSHA256:hashes,imageKeys};})()`)
	if e != nil {
		return e
	}
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("主题预备结果无效")
	}
	definition, _ := m["definition"].(map[string]any)
	raw, _ := definition["themes"].(string)
	i := strings.IndexByte(raw, '{')
	if i < 0 {
		return fmt.Errorf("主题缺少原始定义")
	}
	var expected map[string]any
	if e = json.NewDecoder(strings.NewReader(raw[i:])).Decode(&expected); e != nil {
		return fmt.Errorf("主题原始定义无法解析：%w", e)
	}
	if str(expected, "name") == "" || expected["themeElements"] == nil {
		return fmt.Errorf("主题原始元素不完整")
	}
	count, ok := m["layoutCount"].(float64)
	if !ok || count < 1 || count > 100 || count != float64(int(count)) {
		return fmt.Errorf("主题版式数量无效")
	}
	hashes, ok := m["imageSHA256"].([]any)
	if !ok {
		return fmt.Errorf("主题资源哈希缺失")
	}
	for _, v := range hashes {
		hash, ok := v.(string)
		if !ok || len(hash) != 64 {
			return fmt.Errorf("主题资源哈希无效")
		}
		s.Images = append(s.Images, hash)
	}
	s.Source = map[string]string{}
	for _, k := range []string{"masters", "layouts", "themes"} {
		value, ok := definition[k].(string)
		if !ok || value == "" {
			return fmt.Errorf("官方主题源码不完整")
		}
		s.Source[k] = value
	}
	keys, ok := m["imageKeys"].(map[string]any)
	if !ok {
		return fmt.Errorf("主题图片身份映射缺失")
	}
	s.ImageKeys = map[string]string{}
	for k, v := range keys {
		hash, ok := v.(string)
		if !ok || len(hash) != 64 {
			return fmt.Errorf("主题图片身份哈希无效")
		}
		s.ImageKeys[k] = hash
	}
	s.Expected = expected
	s.LayoutCount = int(count)
	return phase3PrepareThemeDelta(s, m["decoded"], m["currentLayouts"])
}
func phase3SlideThemeNativeJS() string {
	return `(()=>{const sdk=window.presentationSDK,wb=sdk?.editor?.workbench;if(wb?.isReady()!==true)return {ok:false,error:'幻灯片未就绪'};const serialize=t=>{if(!t||typeof t.themeElements?.toJSON!=='function')throw Error('主题完整读回接口缺失');const out={name:t.name,themeElements:t.themeElements.toJSON()},defaults={};for(const k of ['spDef','lnDef','txDef'])if(t[k]){if(typeof t[k].toJSON!=='function')throw Error('主题默认样式读回接口缺失');defaults[k]=t[k].toJSON()}if(Object.keys(defaults).length)out.objectDefaults=defaults;if(t.extraClrSchemeLst?.length)out.extraClrSchemeLst=t.extraClrSchemeLst.map(x=>x.toJSON());return out};return {ok:true,slides:sdk.API.slides.getSlides().map((s,i)=>{const layout=wb.editor.presentation.slides[i]?.layout,master=layout?._$master$_;if(!master||!master.layouts.includes(layout))throw Error('页面母版/版式关系无效');return {id:s.id,themeId:master.themeId,layoutCount:master.layouts.length,theme:serialize(master.theme)}})}})()`
}
func phase3VerifySlideThemeNative(v any, ids []string, s phase3SlideThemeSpec) error {
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("主题读回无效")
	}
	slides, ok := m["slides"].([]any)
	if !ok || len(slides) != len(ids) {
		return fmt.Errorf("主题页面数量不符")
	}
	for i, v := range slides {
		slide, ok := v.(map[string]any)
		if s.SlideLayouts != nil {
			index, found := s.SlideLayouts[ids[i]]
			if !found || slide["layoutIndex"] != float64(index) {
				return fmt.Errorf("第%d页未使用源码匹配的目标版式", i+1)
			}
		}
		if !ok || str(slide, "id") != ids[i] || str(slide, "themeId") != s.ID || slide["layoutCount"] != float64(s.LayoutCount) || !reflect.DeepEqual(phase3ThemeNativeDefaults(slide["theme"]), phase3ThemeNativeDefaults(s.Expected)) {
			return fmt.Errorf("第%d页完整主题、官方标识或版式数量未持久化", i+1)
		}
	}
	return nil
}
func phase3SlideThemeMutationJS(s phase3SlidesSpec, ids []string) string {
	id, _ := json.Marshal(s.Theme.ID)
	list, _ := json.Marshal(ids)
	return `(()=>{const id=` + string(id) + `,expected=` + string(list) + `;` + phase3PresenceGuardJS() + `
const sdk=window.presentationSDK,wb=sdk?.editor?.workbench,staged=window.__shimoPhase3SlideTheme;if(wb?.isReady()!==true||wb.permissions?.canEdit?.()!==true)return {ok:false,error:'未确认编辑权限'};if(JSON.stringify(sdk.API.slides.getSlides().map(s=>s.id))!==JSON.stringify(expected)||staged?.id!==id)return {ok:false,error:'页面变化或缺少已预备主题'};if(wb.editor.presentation.slideMasters.some(m=>m.themeId===id))return {ok:false,error:'主题母版已出现，禁止重复应用'};wb.core.applyOfficalTheme(wb.editor,{id,theme:JSON.stringify(staged.definition),images:staged.images,type:'all'});delete window.__shimoPhase3SlideTheme;return ` + phase3SlideThemeNativeJS() + `;})()`
}
