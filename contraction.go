package main

import "strings"

var (
	elipsis = "…"
	e       = len(strings.Split(elipsis, ""))
)

func Contract(in string, total, suffix int) string {
	inUnicode := strings.Split(in, "")
	i := len(inUnicode)
	if i <= total {
		return in
	}

	outUnicode := inUnicode[:total-suffix-e]
	outUnicode = append(outUnicode, elipsis)
	outUnicode = append(outUnicode, in[i-suffix:])
	return strings.Join(outUnicode, "")
	//return in[:total-suffix-e] + elipsis + in[i-suffix:]
}
