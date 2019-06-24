package main

import (
	"strings"
	"log"
	"encoding/json"
	"encoding/hex"
	"crypto/sha1"
	"net/http"
	"io"
	"os"
	"bufio"
)

type Result struct {
	URL		string	`json:"url"`
	SHA1	string	`json:"sha1"`
}

func main() {
	fi, err := os.Open("input.txt")
	if err != nil {
		panic(err)
	}
	defer fi.Close()
	bfi := bufio.NewReader(fi)

	var results []Result

	buff := make([]byte, 32 * 1024)
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
		defer hres.Body.Close()

		hash := sha1.New()
		for {
			read, err := hres.Body.Read(buff)
			if err != nil && err != io.EOF {
				panic(err)
			}
			if read == 0 {
				break
			}
			hash.Write(buff[:read])
		}

		r := Result {
			URL		: line,
			SHA1	: hex.EncodeToString(hash.Sum(nil)),
		}
		results = append(results, r)
	}

	
	fo, err := os.OpenFile("output.txt", os.O_CREATE, 660)
	if err != nil {
		panic(err)
	}
	defer fo.Close()
	fo.Truncate(0)
	
	enc := json.NewEncoder(fo)
	enc.SetIndent("", "\t")
	err = enc.Encode(&results)
	if err != nil {
		panic(err)
	}
}