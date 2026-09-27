package rtf

import (
	"strconv"
	"strings"
)

// PlainText extracts the visible text from RTF, well enough for previews.
// Destination groups (font/color tables, {\*...}) are skipped, \par becomes
// a space, and \uN escapes are decoded.
func PlainText(data []byte) string {
	s := string(data)
	var out strings.Builder
	type group struct{ skip bool }
	stack := []group{{}}
	skipNext := 0 // fallback characters to drop after a \uN
	skipDest := map[string]bool{"fonttbl": true, "colortbl": true, "stylesheet": true, "info": true, "pict": true}

	for i := 0; i < len(s); i++ {
		c := s[i]
		top := &stack[len(stack)-1]
		switch c {
		case '{':
			stack = append(stack, group{skip: top.skip})
			if strings.HasPrefix(s[i+1:], `\*`) {
				stack[len(stack)-1].skip = true
			}
		case '}':
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		case '\\':
			if i+1 >= len(s) {
				break
			}
			n := s[i+1]
			if n == '\\' || n == '{' || n == '}' {
				if !top.skip {
					out.WriteByte(n)
				}
				i++
				break
			}
			if n == '\'' && i+3 < len(s) { // \'hh: one byte in the ANSI code page
				if v, err := strconv.ParseUint(s[i+2:i+4], 16, 8); err == nil && !top.skip {
					if skipNext > 0 {
						skipNext--
					} else {
						out.WriteRune(rune(v)) // Latin-1 approximation of cp1252
					}
				}
				i += 3
				break
			}
			// control word: letters, optional signed number, optional space
			j := i + 1
			for j < len(s) && isLetter(s[j]) {
				j++
			}
			word := s[i+1 : j]
			k := j
			if k < len(s) && (s[k] == '-' || isDigit(s[k])) {
				k++
				for k < len(s) && isDigit(s[k]) {
					k++
				}
			}
			arg := s[j:k]
			if k < len(s) && s[k] == ' ' {
				k++
			}
			i = k - 1
			switch {
			case skipDest[word]:
				top.skip = true
			case top.skip:
			case word == "par" || word == "line":
				out.WriteByte(' ')
			case word == "u":
				if v, err := strconv.Atoi(arg); err == nil {
					out.WriteRune(rune(uint16(int16(v))))
					skipNext = 1
				}
			}
		case '\r', '\n':
		default:
			if top.skip {
				break
			}
			if skipNext > 0 {
				skipNext--
				break
			}
			out.WriteByte(c)
		}
	}
	return strings.Join(strings.Fields(out.String()), " ")
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isDigit(c byte) bool  { return c >= '0' && c <= '9' }
