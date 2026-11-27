package main

import (
	"fmt"
	"time"
)

// Opening a menu and locating an option does not format the selection. Check
// every requested value first so an unavailable palette color cannot leave an
// earlier font/size change behind. Re-select the target after this UI probe.
func (b *phase3EditorBrowser) preflightLightDocAttrs(spec lightDocFormatSpec) error {
	for _, key := range []string{"fontFamily", "fontSize", "color", "highlight"} {
		value, ok := spec.Attrs[key]
		if !ok {
			continue
		}
		b.pressEscape()
		var err error
		text, color := fmt.Sprint(value), ""
		switch key {
		case "fontFamily":
			err = b.clickControl("", "字体", "PARAGRAPH_STYLE", "", 1)
		case "fontSize":
			err = b.clickControl("", "FONTSIZE", "", "", 0)
		case "color":
			err = b.clickControl("", "", "SvgCommonFontColor", "", 1)
			text, color = "", fmt.Sprint(value)
		case "highlight":
			err = b.clickControl("", "", "SvgCommonFontHighlight", "", 1)
			text, color = "", fmt.Sprint(value)
		}
		if err != nil {
			return fmt.Errorf("%s 写前选项检查失败，未应用任何字符格式：%w", key, err)
		}
		time.Sleep(900 * time.Millisecond)
		_, err = b.Eval(lightDocPopoverPickJS(text, color, ""))
		b.pressEscape()
		if err != nil {
			return fmt.Errorf("%s=%v 不在当前页面可用选项中，未应用任何字符格式：%w", key, value, err)
		}
	}
	return nil
}
