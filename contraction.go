package main

var (
	elipsis = "..."
	e       = len(elipsis)
)

func Contract(in string, total, suffix int) string {
	i := len(in)
	if i <= total {
		return in
	}

	return in[:total-suffix-e] + elipsis + in[i-suffix:]
}
