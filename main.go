package main

import (
	"os"
	"strings"

	"twimgdns/src/server"
	"twimgdns/src/tester"
)

func main() {
	for _, arg := range os.Args {
		switch {
		case strings.EqualFold(arg, "--server"):
			server.Main()

		case strings.EqualFold(arg, "--tester"):
			tester.Main()
		}
	}
}
