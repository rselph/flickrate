package main

import (
	"fmt"
	"os/exec"
)

func openInBrowser(url string) (err error) {
	cmd := exec.Command("open", url)
	if verbose {
		fmt.Println(cmd.Args)
	}
	err = cmd.Run()
	return
}
