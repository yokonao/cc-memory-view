package memory

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

var hiddenNames = map[rune]string{
	0x00AD: "soft hyphen",
	0x061C: "arabic letter mark",
	0x115F: "hangul choseong filler",
	0x1160: "hangul jungseong filler",
	0x180E: "mongolian vowel separator",
	0x200B: "zero width space",
	0x200C: "zero width non-joiner",
	0x200D: "zero width joiner",
	0x200E: "left-to-right mark",
	0x200F: "right-to-left mark",
	0x202A: "left-to-right embedding",
	0x202B: "right-to-left embedding",
	0x202C: "pop directional formatting",
	0x202D: "left-to-right override",
	0x202E: "right-to-left override",
	0x2060: "word joiner",
	0x2061: "function application",
	0x2062: "invisible times",
	0x2063: "invisible separator",
	0x2064: "invisible plus",
	0x2066: "left-to-right isolate",
	0x2067: "right-to-left isolate",
	0x2068: "first strong isolate",
	0x2069: "pop directional isolate",
	0x3164: "hangul filler",
	0xFEFF: "zero width no-break space",
	0xFFA0: "halfwidth hangul filler",
}

func hiddenName(r rune) string {
	switch {
	case hiddenNames[r] != "":
		return hiddenNames[r]
	case r >= 0xE0000 && r <= 0xE007F:
		return "tag character"
	case isVariationSelector(r):
		return "variation selector"
	case unicode.IsControl(r):
		return "control character"
	default:
		return "format character"
	}
}

func isVariationSelector(r rune) bool {
	return r >= 0xFE00 && r <= 0xFE0F || r >= 0xE0100 && r <= 0xE01EF
}

// isHidden reports whether rs[i] is invisible and not part of ordinary
// text: joiners between non-ASCII letters or emoji, emoji tag sequences, a
// single variation selector and a leading BOM are left alone.
func isHidden(rs []rune, i int) bool {
	r := rs[i]
	prev, next := rune(0), rune(0)
	if i > 0 {
		prev = rs[i-1]
	}
	if i+1 < len(rs) {
		next = rs[i+1]
	}
	switch {
	case r == '\t' || r == '\n' || r == '\r':
		return false
	case r == 0xFEFF:
		return i != 0
	case r == 0x200C || r == 0x200D:
		return prev <= unicode.MaxASCII || next <= unicode.MaxASCII
	case isVariationSelector(r):
		return isVariationSelector(prev) || isVariationSelector(next)
	case r >= 0xE0000 && r <= 0xE007F:
		return !inEmojiTagSequence(rs, i)
	case hiddenNames[r] != "", unicode.IsControl(r):
		return true
	}
	return unicode.Is(unicode.Cf, r) && !unicode.Is(unicode.Prepended_Concatenation_Mark, r)
}

// inEmojiTagSequence reports whether rs[i] is in a flag tag sequence such as
// the flag of Scotland: U+1F3F4, tag letters, then U+E007F.
func inEmojiTagSequence(rs []rune, i int) bool {
	start := i
	for start > 0 && rs[start-1] >= 0xE0020 && rs[start-1] <= 0xE007E {
		start--
	}
	if start == 0 || rs[start-1] != 0x1F3F4 {
		return false
	}
	for j := i; j < len(rs); j++ {
		switch {
		case rs[j] == 0xE007F:
			return true
		case rs[j] < 0xE0020 || rs[j] > 0xE007E:
			return false
		}
	}
	return false
}

type hiddenChar struct {
	r         rune
	count     int
	line, col int
}

// hiddenChars returns the hidden characters in s in order of first
// appearance, with where each first appears. Tag characters and variation
// selectors, which can spell out a whole hidden message, count as one rune
// each.
func hiddenChars(s string) []hiddenChar {
	rs := []rune(s)
	var found []hiddenChar
	seen := map[rune]int{}
	line, col := 1, 1
	for i, r := range rs {
		if isHidden(rs, i) {
			key := r
			switch {
			case r >= 0xE0000 && r <= 0xE007F:
				key = 0xE0000
			case isVariationSelector(r):
				key = 0xFE00
			}
			if j, ok := seen[key]; ok {
				found[j].count++
			} else {
				seen[key] = len(found)
				found = append(found, hiddenChar{r: r, count: 1, line: line, col: col})
			}
		}
		if r == '\n' {
			line, col = line+1, 1
		} else {
			col++
		}
	}
	return found
}

func (h hiddenChar) String() string {
	name := hiddenName(h.r)
	if h.count > 1 {
		name += "s"
	}
	return fmt.Sprintf("%d %s, first U+%04X at line %d:%d", h.count, name, h.r, h.line, h.col)
}

var markupTags = []string{
	"system-reminder", "command-name", "command-message", "command-args",
	"local-command-stdout", "local-command-stderr", "local-command-caveat",
	"user-prompt-submit-hook", "bash-input", "bash-stdout", "bash-stderr",
	"task-notification", "persisted-output",
	"functions", "function_calls", "function_results",
}

var markupRe = regexp.MustCompile(`</?((?:antml:)?[a-zA-Z][\w:-]*)[^<>]*>`)

type markupTag struct {
	tag   string
	count int
	line  int
}

// markup returns the tags in s that imitate Claude Code's own markup, one per
// tag name in order of first appearance.
func markup(s string) []markupTag {
	var found []markupTag
	seen := map[string]int{}
	for _, loc := range markupRe.FindAllStringSubmatchIndex(s, -1) {
		name := s[loc[2]:loc[3]]
		if !strings.HasPrefix(name, "antml:") && !slices.Contains(markupTags, name) {
			continue
		}
		tag := "<" + name + ">"
		if j, ok := seen[tag]; ok {
			found[j].count++
			continue
		}
		seen[tag] = len(found)
		found = append(found, markupTag{tag: tag, count: 1, line: strings.Count(s[:loc[0]], "\n") + 1})
	}
	return found
}
