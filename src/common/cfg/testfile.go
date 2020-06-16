package cfg

import (
	"encoding/csv"
	"encoding/hex"
	"io"
	"net/url"
	"os"
)

const (
	testFilePath = "./config-testfile.csv"
)

type TestDataMap map[string][]byte

var TestFile = make(map[string]TestDataMap)

func init() {
	fs, err := os.Open(testFilePath)
	if err != nil {
		panic(err)
	}
	defer fs.Close()

	r := csv.NewReader(fs)

	for {
		r, err := r.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			panic(err)
		}

		u, err := url.Parse(r[0])
		if err != nil {
			panic(err)
		}

		if _, ok := TestFile[u.Host]; !ok {
			TestFile[u.Host] = make(TestDataMap)
		}

		b, err := hex.DecodeString(r[1])
		if err != nil {
			panic(err)
		}
		TestFile[u.Host][r[0]] = b
	}
}
