package main

import (
	"fmt"
	"os/exec"
)

func openInBrowser(url string) (err error) {
	cmd := exec.Command("cmd.exe", "/C", "start", url)
	if verbose {
		fmt.Println(cmd.Args)
	}
	err = cmd.Run()
	return
}
