package main

import (
	"flag"
	"fmt"
	"os"
	"time"
)

var version = "0.0.0"

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Printf("click version %s\n", version)
		return
	}
	sleep := flag.Bool("sleep", false, "sleep after reporting")
	flag.Parse()
	if *sleep {
		time.Sleep(10 * time.Second)
	}
}
