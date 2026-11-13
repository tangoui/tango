//go:build !js

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

func main() {
	catalog := flag.Bool("catalog", false, "print registered live gallery pages as JSON")
	flag.Parse()
	if *catalog {
		if err := json.NewEncoder(os.Stdout).Encode(livePages()); err != nil {
			panic(err)
		}
		return
	}
	fmt.Println("Build the browser playground with: npm --prefix site run docs:playground")
}
