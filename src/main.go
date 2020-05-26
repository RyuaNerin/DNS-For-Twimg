package src

import (
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

var (
	logV *log.Logger = func() *log.Logger {
		var verbose bool
		for _, s := range os.Args {
			if strings.EqualFold(s, "--verbose") {
				verbose = true
			}
		}
		if verbose {
			return log.New(os.Stdout, "", log.LstdFlags)
		} else {
			return log.New(ioutil.Discard, "", log.LstdFlags)
		}
	}()
)

func Main() {
	l := startHttpServer()
	defer l.Close()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-sig
}
