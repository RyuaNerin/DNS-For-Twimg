package src

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"twimgdns/src/cfg"

	"github.com/getsentry/sentry-go"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

func Main() {
	router := gin.New()

	router.Use(handlePanic)

	pprof.Register(router)

	router.GET("/json", httpJson.Handler)
	router.GET("/json.2", httpJson2.Handler)

	router.Static("/static/", "public/static/")
	router.GET("/", func(ctx *gin.Context) {
		ctx.File("public/index.htm")
	})

	server := http.Server{
		ErrorLog: log.New(ioutil.Discard, "", 0),
		Handler:  router,
	}

	listener, err := net.Listen(cfg.V.HTTP.Server.ListenType, cfg.V.HTTP.Server.Listen)
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	go func() {
		err = server.Serve(listener)
		if err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-sig
}

func handlePanic(ctx *gin.Context) {
	defer func() {
		if err := recover(); err != nil {
			var brokenPipe bool
			if ne, ok := err.(*net.OpError); ok {
				if se, ok := ne.Err.(*os.SyscallError); ok {
					if strings.Contains(strings.ToLower(se.Error()), "broken pipe") || strings.Contains(strings.ToLower(se.Error()), "connection reset by peer") {
						brokenPipe = true
					}
				}
			}

			if brokenPipe {
				ctx.Error(err.(error))
				ctx.Abort()
			} else {
				fmt.Printf("%+v", errors.WithStack(err.(error)))
				sentry.CaptureException(err.(error))
				ctx.Abort()
			}
		}
	}()
	ctx.Next()
}
