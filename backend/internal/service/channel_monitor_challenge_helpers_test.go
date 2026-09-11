//go:build unit

package service

import (
	"regexp"
	"strconv"
)

var monitorChallengeExprRegex = regexp.MustCompile(`(\d+)\s*([+-])\s*(\d+)`)

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
