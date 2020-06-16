package common

import (
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

var (
	Verbose *log.Logger = func() *log.Logger {
		for _, s := range os.Args {
			if strings.EqualFold(s, "--verbose") {
				return log.New(os.Stdout, "", log.LstdFlags)
			}
		}

		gin.SetMode(gin.ReleaseMode)
		return log.New(ioutil.Discard, "", 0)
	}()
)
