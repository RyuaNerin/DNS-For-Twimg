package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	//"net/url"
	"strings"
	"time"
	
	//"github.com/garyburd/go-oauth/oauth"
)

func main() {
	loadConfig()

	mux := &http.ServeMux{}
	mux.Handle("/resources/"	, http.FileServer(http.Dir("resources")))
	mux.Handle("/json"			, http.HandlerFunc(httpJSONHandler))
	mux.Handle("/"				, http.HandlerFunc(httpIndexHandler))

    server := http.Server {
		ErrorLog		: log.New(ioutil.Discard, "", 0),
		Handler			: mux,
        ReadTimeout		: config.HTTP.TimeoutRead .Duration,
        WriteTimeout	: config.HTTP.TimeoutWrite.Duration,
        IdleTimeout		: config.HTTP.TimeoutIdle .Duration,
	}

	listener, err := net.Listen(config.HTTP.Type, config.HTTP.Listen)
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
	var cdnTestResult CdnStatusCollection

	if !cdnTestResult.TestCdn() {
		return
	}
	
	setHTTPPage(cdnTestResult)
	setDNSHostIP(cdnTestResult)

	{
		var sb strings.Builder
		sb.WriteString("CdnResults Updated")
		
		for host, cdn := range cdnTestResult {
			sb.WriteString(fmt.Sprintf("%s : %s (Total %d Cdn)\n", host, cdn[0].IP.String(), len(cdn)))
		}

		log.Println(sb.String())
	}

	/*
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
	*/
}