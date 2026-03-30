package main

import "fmt"

// Version is the semantic version of aitop. It defaults to "0.0.0-dev" for
// local builds and is overridden at release time via:
//
//	go build -ldflags "-X main.Version=1.2.3" ./cmd/aitop
var Version = "0.0.0-dev"

func runVersion() {
	fmt.Printf("aitop version %s\n", Version)
}
