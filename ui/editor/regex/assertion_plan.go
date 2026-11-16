package regex

import "strings"

type asciiAssertionPlan struct {
	consume  []byte
	check    []byte
	positive bool
}

func compileASCIIAssertionPlan(source, flags string) *asciiAssertionPlan {
	if flags != "" {
		return nil
	}
	for _, marker := range []struct {
		text     string
		positive bool
	}{
		{"(?=", true},
		{"(?!", false},
	} {
		i := strings.Index(source, marker.text)
		if i <= 0 || !strings.HasSuffix(source, ")") || strings.Index(source[i+len(marker.text):len(source)-1], "(") >= 0 {
			continue
		}
		consume, ok := parseASCIILiteral(source[:i])
		if !ok || len(consume) == 0 {
			continue
		}
		check, ok := parseASCIILiteral(source[i+len(marker.text) : len(source)-1])
		if !ok || len(check) == 0 {
			continue
		}
		return &asciiAssertionPlan{consume: consume, check: check, positive: marker.positive}
	}
	return nil
}

func parseASCIILiteral(source string) ([]byte, bool) {
	out := make([]byte, 0, len(source))
	for i := 0; i < len(source); i++ {
		c := source[i]
		if c == '\\' {
			if i+1 >= len(source) {
				return nil, false
			}
			i++
			c = source[i]
			if !strings.ContainsRune(`/\\.*+?(){}[]|^$`, rune(c)) {
				return nil, false
			}
		} else if strings.ContainsRune(`.^$*+?(){}[]|`, rune(c)) || c >= 0x80 {
			return nil, false
		}
		out = append(out, c)
	}
	return out, true
}
