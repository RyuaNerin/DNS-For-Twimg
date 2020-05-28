package src

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	"twimgdns/src/cfg"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

func startHttpServer() net.Listener {
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()

	router.Use(handlePanic)

	router.Any("/debug/pprof/", gin.WrapH(http.DefaultServeMux))

	router.Static("/static/", "public/static")
	router.GET("/json", httpJson.Handler)
	router.GET("/json.2", httpJson2.Handler)
	router.GET("/", httpIndex.Handler)

	server := http.Server{
		ErrorLog: log.New(ioutil.Discard, "", 0),
		Handler:  router,
	}

	listener, err := net.Listen(cfg.V.HTTP.Server.ListenType, cfg.V.HTTP.Server.Listen)
	if err != nil {
		panic(err)
	}

	go func() {
		err = server.Serve(listener)
		if err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	return listener
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
