package config

import "bytes"

// stripJSONComments removes // and /* */ comments from JSON input while
// preserving characters inside string literals and respecting backslash
// escapes. Comment characters are replaced with a single space so that token
// positions stay stable for the stdlib JSON decoder.
//
// This is a deliberately small preprocessor suitable for hand-written configs.
// It does not implement the full JSONC specification and does not handle
// Unicode edge cases.
func stripJSONComments(in []byte) []byte {
	in = bytes.TrimSpace(in)
	if len(in) == 0 {
		return in
	}
	out := make([]byte, 0, len(in))
	for i := 0; i < len(in); {
		c := in[i]
		// String literal: copy verbatim, honouring backslash escapes.
		if c == '"' {
			out = append(out, c)
			i++
			for i < len(in) {
				out = append(out, in[i])
				if in[i] == '\\' && i+1 < len(in) {
					out = append(out, in[i+1])
					i += 2
					continue
				}
				if in[i] == '"' {
					i++
					break
				}
				i++
			}
			continue
		}
		// Line comment.
		if c == '/' && i+1 < len(in) && in[i+1] == '/' {
			out = append(out, ' ')
			i += 2
			for i < len(in) && in[i] != '\n' {
				i++
			}
			continue
		}
		// Block comment.
		if c == '/' && i+1 < len(in) && in[i+1] == '*' {
			out = append(out, ' ')
			i += 2
			for i+1 < len(in) && !(in[i] == '*' && in[i+1] == '/') {
				i++
			}
			if i+1 < len(in) {
				i += 2
			} else {
				i = len(in)
			}
			continue
		}
		out = append(out, c)
		i++
	}
	return out
}
