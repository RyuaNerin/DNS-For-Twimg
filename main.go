package main

import (
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"time"
	
	"github.com/garyburd/go-oauth/oauth"
)

func main() {
	mux := &http.ServeMux{}
	mux.Handle("/resources/", http.FileServer(http.Dir("resources")))
	mux.Handle("/", http.HandlerFunc(httpHandler))

    server := http.Server {
		ErrorLog : log.New(ioutil.Discard, "", 0),
		Handler: mux,
        ReadTimeout : 30 * time.Second,
        WriteTimeout : 30 * time.Second,
        IdleTimeout : 30 * time.Second,
	}

	var listener net.Listener
	var err error
    if runtime.GOOS == "windows" {
		listener, err = net.Listen("tcp", ":8080")
	} else {
		listener, err = net.Listen("unix", "/run/twimg-cdn-status.socket")
	}

	if err != nil {
		panic(err)
	}

	go startDNSServer()
	go refreshCdn()

	err = server.Serve(listener)
	if err != nil {
		panic(err)
	}
}

func refreshCdn() {
	for {
		nextTime := time.Now().Truncate(time.Hour)
		nextTime = nextTime.Add(time.Hour)

		go refreshCdnWorker()

		time.Sleep(time.Until(nextTime))
	}
}

func refreshCdnWorker() {
	var cdnTester cdnTester

	if !cdnTester.TestCdn() {
		return
	}
	
	setCdnResults(cdnTester.CdnDefualt, cdnTester.CdnList)
	log.Printf("CdnResults Updated (count : %d)\n", len(cdnTester.CdnList))

	if len(cdnTester.CdnList) > 0 {
		setCdnBest(cdnTester.CdnList[0].IP)
	}

	oauthClient := oauth.Client {
		Credentials : oauth.Credentials {
			Token : "",
			Secret : "",
		},
		Header : make(http.Header),
	}
	userToken := oauth.Credentials{
		Token: "",
		Secret : "",
	}	
	oauthClient.Header.Set("Accept-Encoding", "gzip, defalte")

	postData := url.Values {}
	postData.Set("status", "")

	resp, err := oauthClient.Post(http.DefaultClient, &userToken, "https://api.twitter.com/1.1/statuses/update.json", postData)
	if err == nil {
		resp.Body.Close()
	}
}