package src

import (
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

var (
	logV *log.Logger = func() *log.Logger {
		var verbose bool
		for _, s := range os.Args {
			if strings.EqualFold(s, "--verbose") {
				verbose = true
				break
			}
		}

		if verbose {
			gin.SetMode(gin.ReleaseMode)
			return log.New(os.Stdout, "", log.LstdFlags)
		} else {
			return log.New(ioutil.Discard, "", 0)
		}
	}()
)
