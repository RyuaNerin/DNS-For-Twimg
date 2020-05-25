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
	logV *log.Logger
)

func Main() {
	var verbose bool
	for _, s := range os.Args {
		if strings.EqualFold(s, "--verbose") {
			verbose = true
		}
	}
	if verbose {
		logV = log.New(os.Stdout, "", log.LstdFlags)
	} else {
		logV = log.New(ioutil.Discard, "", log.LstdFlags)
	}

	loadLastTestResult()

	go testCdnWorker()
	go statLogWorker()

	////////////////////////////////////////////////////////////////////////////////////////////////////

	l := startHttpServer()
	defer l.Close()

	////////////////////////////////////////////////////////////////////////////////////////////////////

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-sig
}
