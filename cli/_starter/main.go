package main

import "github.com/callumeddisford/barry"

func main() {
	barry.Start(barry.RuntimeConfig{
		Env:         "dev",
		EnableCache: false,
		Port:        8080,
	})
}
