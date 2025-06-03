package main

import "strings"

func joinPipe(v []string) (s string) {
	s = strings.Join(v, "|")
	s = strings.ReplaceAll(s, ",", "|")
	return
}
