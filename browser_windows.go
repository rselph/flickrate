package main

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
)

func openInBrowser(url string) (err error) {
	cmd := exec.Command("cmd.exe", "/C", "start", winEscape(url))
	if verbose {
		fmt.Println(cmd.Args)
	}
	outBytes, err := cmd.CombinedOutput()
	if err != nil {
		log.Println(string(outBytes))
	}
	return
}

func winEscape(in string) (out string) {
	out = strings.ReplaceAll(in, `&`, `^&`)
	return
}
