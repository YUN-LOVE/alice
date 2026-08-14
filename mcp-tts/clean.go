package main

import (
	"regexp"
	"strings"
)

// ==================== TTS 文本清洗 ====================
// 去掉"无法发声的语素"：括号内容（动作/语气修饰）、颜文字、表情符号串。
// Alice 的回复常带（小声嘟囔）（盯）(｡･ω･｡) 哼╯^╰ 等，朗读时会被读出来，
// 合成前统一清洗。

// 括号及内容（全角/半角，一层嵌套内；迭代处理嵌套）
var reParen = regexp.MustCompile(`[（(][^（()）]*[)）]`)

// 连续 2+ 个"非文字符号"（排除汉字/字母/数字/空白/常见标点）——颜文字残留等
// 白名单标点保留：，。！？、；：""''——…·~.!?,- 以及括号本身
var reSymbolRun = regexp.MustCompile(
	`[^\p{Han}\p{L}\p{N}\s，。！？、；：""''（）()——…·~.!?,;-]{2,}`,
)

// 单个无意义符号残留（如 ╯ 单独出现）
var reLoneSymbol = regexp.MustCompile(
	`[^\p{Han}\p{L}\p{N}\s，。！？、；：""''（）()——…·~.!?,;-]`,
)

// cleanTTS 清洗文本中无法发声的语素
func cleanTTS(text string) string {
	// 1. 迭代去除括号及内容（支持嵌套，最多 3 层）
	for i := 0; i < 3; i++ {
		cleaned := reParen.ReplaceAllString(text, "")
		if cleaned == text {
			break
		}
		text = cleaned
	}
	// 2. 去除连续符号串（颜文字等）
	text = reSymbolRun.ReplaceAllString(text, " ")
	// 3. 去除残留的单个符号（清理后不再是连续串的孤例）
	text = reLoneSymbol.ReplaceAllString(text, "")
	// 4. 合并空白
	text = strings.Join(strings.Fields(text), " ")
	return strings.TrimSpace(text)
}
