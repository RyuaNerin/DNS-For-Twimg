package cfg

import (
	"encoding/hex"
	"io/ioutil"
	"math/rand"
	"strings"
)

var UpdateHeaderValue string

func init() {
	pw, err := ioutil.ReadFile("config.auth")
	if err == nil {
		UpdateHeaderValue = strings.TrimSpace(string(pw))

	} else {
		pw := make([]byte, 32)
		rand.Read(pw)
		UpdateHeaderValue = hex.EncodeToString(pw)

		err := ioutil.WriteFile("config.auth", []byte(UpdateHeaderValue), 0400)
		if err != nil {
			panic(err)
		}
	}
}
