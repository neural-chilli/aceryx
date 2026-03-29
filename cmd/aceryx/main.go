package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("aceryx v0.0.1-dev")
		return
	}

	fmt.Println("aceryx - case orchestration engine")
	fmt.Println("usage: aceryx [serve|version]")
}
