package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

type Result struct {
	URL  string `json:"url"`
	SHA1 string `json:"sha1"`
}

func main() {
	fi, err := os.Open("input.txt")
	if err != nil {
		panic(err)
	}
	defer fi.Close()
	bfi := bufio.NewReader(fi)

	fo, err := os.OpenFile("output.txt", os.O_CREATE, 660)
	if err != nil {
		panic(err)
	}
	defer fo.Close()
	fo.Truncate(0)
	bfo := bufio.NewWriter(fo)

	for {
		line, err := bfi.ReadString('\n')
		if err != nil && err != io.EOF {
			panic(err)
		}
		line = strings.TrimSuffix(line, "\n")
		log.Println(line)
		if line == "" {
			break
		}

		hres, err := http.Get(line)
		if err != nil {
			panic(err)
		}

		h1 := sha256.New()
		_, err = io.Copy(h1, hres.Body)
		hres.Body.Close()
		if err != nil && err != io.EOF {
			panic(err)
		}

		fmt.Fprintf(bfo, "\"%s\": \"%s\",\n", line, hex.EncodeToString(h1.Sum(nil)))
	}

	bfo.Flush()
}
