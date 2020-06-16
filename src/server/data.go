package server

import (
	"bufio"
	"html/template"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
	"unsafe"

	"twimgdns/src/common"
	"twimgdns/src/common/cfg"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	jsoniter "github.com/json-iterator/go"
)

const timeFormat = "2006-01-02 15:04 (Z07:00)"

var zoneTemplate = template.Must(template.ParseFiles("template/zone.tmpl"))

func init() {
	jsoniter.RegisterTypeEncoderFunc(
		"time.Time",
		func(ptr unsafe.Pointer, stream *jsoniter.Stream) {
			stream.WriteString((*time.Time)(ptr).Format(timeFormat))
		},
		nil,
	)

	jsoniter.RegisterTypeDecoderFunc(
		"time.Time",
		func(ptr unsafe.Pointer, iter *jsoniter.Iterator) {
			s := iter.ReadString()
			t, err := time.Parse(timeFormat, s)
			if err != nil {
				iter.ReportError("time.Time", err.Error())
				return
			}

			*(*time.Time)(ptr) = t
		},
	)

	fs, err := os.Open(cfg.V.Path.TestSave)
	if err == nil {
		defer fs.Close()

		var data common.Result
		err = jsoniter.NewDecoder(fs).Decode(&data)
		if err != nil {
			panic(err)
		}

		setHttpJsonData(data)
	}
}

func handleUpdateNewData(ctx *gin.Context) {
	defer ctx.Status(http.StatusOK)

	if ctx.GetHeader(common.UpdateHeaderName) != cfg.UpdateHeaderValue {
		return
	}

	var data common.Result
	err := jsoniter.NewDecoder(ctx.Request.Body).Decode(&data)
	if err != nil && err != io.EOF {
		return
	}

	go saveResultData(data)
	go setHttpJsonData(data)
}

func saveResultData(data common.Result) {
	os.MkdirAll(filepath.Dir(cfg.V.Path.TestSave), 0700)

	fsSave, err := os.OpenFile(cfg.V.Path.TestSave, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 640)
	if err != nil {
		sentry.CaptureException(err)
		return
	}
	defer fsSave.Close()

	fsSave.Truncate(0)
	fsSave.Seek(0, 0)

	bw := bufio.NewWriter(fsSave)

	err = jsoniter.NewEncoder(bw).Encode(data)
	if err != nil {
		sentry.CaptureException(err)
		return
	}
	bw.Flush()
	fsSave.Close()

	////////////////////////////////////////////////////////////////////////////////////////////////////
	os.MkdirAll(filepath.Dir(cfg.V.Path.ZoneFile), 0700)

	fsZone, err := os.OpenFile(cfg.V.Path.ZoneFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0700)
	if err != nil {
		sentry.CaptureException(err)
		return
	}
	defer fsZone.Close()

	fsSave.Truncate(0)
	fsSave.Seek(0, 0)

	bw.Reset(fsZone)

	var td struct {
		Serial string
		Data   common.Result
	}
	td.Serial = time.Now().Format("0601021504")
	td.Data = data

	err = zoneTemplate.Execute(bw, &td)
	if err != nil {
		sentry.CaptureException(err)
		return
	}

	bw.Flush()
	fsZone.Close()

	////////////////////////////////////////////////////////////////////////////////////////////////////

	cmd := exec.Command("rndc", "reload")
	if cmd.Start() != nil {
		cmd.Wait()
	}

	cmd = exec.Command("rndc", "flush", "dynamic")
	if cmd.Start() != nil {
		cmd.Wait()
	}
}
