package service

import (
	"fmt"
	"math/rand/v2"
	"regexp"
	"strconv"
)

// monitorChallengePromptTemplates 短随机算术题。故意不用 few-shot：
// 少 token、少固定指纹，每次测活只换数字和句式。
var monitorChallengePromptTemplates = []string{
	`%d%s%d`,
	`%d %s %d`,
	`%d%s%d=`,
	`%d %s %d =`,
	`%d %s %d = ?`,
}

// monitorChallengeNumberRegex 提取响应中的所有整数（含负号）。
var monitorChallengeNumberRegex = regexp.MustCompile(`-?\d+`)

// monitorChallengeExprRegex 从短 prompt 里抽出算术式，供测试与校验复用。
var monitorChallengeExprRegex = regexp.MustCompile(`(\d+)\s*([+-])\s*(\d+)`)

// monitorChallenge 一次 challenge 的 prompt + 期望答案。
type monitorChallenge struct {
	Prompt   string
	Expected string
}

// generateChallenge 生成一次随机算术 challenge：
//   - 随机两个 [monitorChallengeMin, monitorChallengeMax] 整数
//   - 50% 加 / 50% 减；减法用 max - min 保证非负
//   - 从短句式池里随机挑一条，避免固定 few-shot 指纹
//
// 不强求加密随机：math/rand/v2 足够分散，避免 crypto/rand 的开销。
func generateChallenge() monitorChallenge {
	a := randIntInRange(monitorChallengeMin, monitorChallengeMax)
	b := randIntInRange(monitorChallengeMin, monitorChallengeMax)
	op := "+"
	expected := a + b
	if rand.IntN(2) != 0 { //nolint:gosec // 仅用于生成测试问题，无安全影响
		hi, lo := a, b
		if lo > hi {
			hi, lo = lo, hi
		}
		a, b = hi, lo
		op = "-"
		expected = hi - lo
	}

	tpl := monitorChallengePromptTemplates[rand.IntN(len(monitorChallengePromptTemplates))] //nolint:gosec
	return monitorChallenge{
		Prompt:   fmt.Sprintf(tpl, a, op, b),
		Expected: strconv.Itoa(expected),
	}
}

// parseChallengeExpression 从 prompt 中解析出最后一个 "N ± N" 式子的答案。
func parseChallengeExpression(prompt string) (string, bool) {
	matches := monitorChallengeExprRegex.FindAllStringSubmatch(prompt, -1)
	if len(matches) == 0 {
		return "", false
	}
	m := matches[len(matches)-1]
	left, _ := strconv.Atoi(m[1])
	right, _ := strconv.Atoi(m[3])
	if m[2] == "+" {
		return strconv.Itoa(left + right), true
	}
	return strconv.Itoa(left - right), true
}

// randIntInRange 返回 [min, max] 闭区间的随机整数。
func randIntInRange(minVal, maxVal int) int {
	if maxVal <= minVal {
		return minVal
	}
	return minVal + rand.IntN(maxVal-minVal+1) //nolint:gosec
}

// validateChallenge 在响应文本中查找 expected 整数答案，返回是否通过校验。
func validateChallenge(responseText, expected string) bool {
	if responseText == "" || expected == "" {
		return false
	}
	matches := monitorChallengeNumberRegex.FindAllString(responseText, -1)
	for _, m := range matches {
		if m == expected {
			return true
		}
	}
	return false
}
