package native

import (
	"strconv"
	"strings"
)

func expandEscapeSequences(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				sb.WriteByte('\n')
				i++
			case 't':
				sb.WriteByte('\t')
				i++
			case 'r':
				sb.WriteByte('\r')
				i++
			case '\\':
				sb.WriteByte('\\')
				i++
			case 'a':
				sb.WriteByte('\a')
				i++
			case 'b':
				sb.WriteByte('\b')
				i++
			case 'f':
				sb.WriteByte('\f')
				i++
			case 'v':
				sb.WriteByte('\v')
				i++
			case 'x':
				if i+3 < len(s) {
					if v, err := strconv.ParseUint(s[i+2:i+4], 16, 8); err == nil {
						sb.WriteByte(byte(v))
						i += 3
						continue
					}
				}
				sb.WriteByte(s[i])
			default:
				if s[i+1] >= '0' && s[i+1] <= '7' {
					end := i + 2
					for end < len(s) && end < i+4 && s[end] >= '0' && s[end] <= '7' {
						end++
					}
					if v, err := strconv.ParseUint(s[i+1:end], 8, 8); err == nil {
						sb.WriteByte(byte(v))
						i = end - 1
						continue
					}
				}
				sb.WriteByte(s[i])
			}
		} else {
			sb.WriteByte(s[i])
		}
	}
	return sb.String()
}

func expandPrintfFormat(format string, args []string) string {
	var sb strings.Builder
	argIdx := 0
	for i := 0; i < len(format); i++ {
		if format[i] == '\\' && i+1 < len(format) {
			switch format[i+1] {
			case 'n':
				sb.WriteByte('\n')
				i++
			case 't':
				sb.WriteByte('\t')
				i++
			case 'r':
				sb.WriteByte('\r')
				i++
			case '\\':
				sb.WriteByte('\\')
				i++
			case 'a':
				sb.WriteByte('\a')
				i++
			case 'b':
				sb.WriteByte('\b')
				i++
			case 'f':
				sb.WriteByte('\f')
				i++
			case 'v':
				sb.WriteByte('\v')
				i++
			default:
				sb.WriteByte(format[i])
			}
			continue
		}
		if format[i] == '%' && i+1 < len(format) {
			spec := format[i+1]
			arg := ""
			if argIdx < len(args) {
				arg = args[argIdx]
				argIdx++
			}
			switch spec {
			case 's':
				sb.WriteString(arg)
			case 'd':
				if v, err := strconv.Atoi(arg); err == nil {
					sb.WriteString(strconv.Itoa(v))
				} else {
					sb.WriteByte('0')
				}
			case 'f':
				if v, err := strconv.ParseFloat(arg, 64); err == nil {
					sb.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
				} else {
					sb.WriteString("0.000000")
				}
			case '%':
				sb.WriteByte('%')
				argIdx--
			case 'c':
				if len(arg) > 0 {
					sb.WriteByte(arg[0])
				}
			case 'x':
				if v, err := strconv.Atoi(arg); err == nil {
					sb.WriteString(strconv.FormatInt(int64(v), 16))
				}
			case 'o':
				if v, err := strconv.Atoi(arg); err == nil {
					sb.WriteString(strconv.FormatInt(int64(v), 8))
				}
			default:
				sb.WriteByte('%')
				sb.WriteByte(spec)
				argIdx--
			}
			i++
			continue
		}
		sb.WriteByte(format[i])
	}
	return sb.String()
}
