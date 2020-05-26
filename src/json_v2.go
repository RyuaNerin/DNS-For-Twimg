package src

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
	"time"

	"twimgdns/src/cfg"

	jsoniter "github.com/json-iterator/go"
)

var zoneTemplate = template.Must(template.ParseFiles("./zone.tmpl"))

type testResultV2 map[string]testResultData
type testResultData struct {
	Default testResultDataCdn `json:"default"`
	Cache   testResultDataCdn `json:"cache"`
	Best    testResultDataCdn `json:"best"`
}

type testResultDataCdn struct {
	Addr  string        `json:"addr"`
	Ping  time.Duration `json:"ping"`
	Speed float64       `json:"speed"`
}

func (data testResultV2) save() {
	os.MkdirAll(filepath.Dir(cfg.V.Path.TestSave), 0700)

	fsSave, err := os.OpenFile(cfg.V.Path.TestSave, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 640)
	if err != nil {
		return
	}
	defer fsSave.Close()

	fsSave.Truncate(0)
	fsSave.Seek(0, 0)

	bw := bufio.NewWriter(fsSave)

	err = jsoniter.NewEncoder(bw).Encode(data)
	if err != nil {
		return
	}
	bw.Flush()
	fsSave.Close()

	////////////////////////////////////////////////////////////////////////////////////////////////////
	os.MkdirAll(filepath.Dir(cfg.V.Path.ZoneFile), 0700)

	fsZone, err := os.OpenFile(cfg.V.Path.ZoneFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0700)
	if err != nil {
		return
	}
	defer fsZone.Close()

	fsSave.Truncate(0)
	fsSave.Seek(0, 0)

	bw.Reset(fsZone)

	var td struct {
		Serial string
		Data   testResultV2
	}
	td.Serial = time.Now().Format("200601021504")
	td.Data = data

	err = zoneTemplate.Execute(bw, &td)
	if err != nil {
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

func init() {
	fs, err := os.Open(cfg.V.Path.TestSave)
	if err != nil {
		return
	}
	defer fs.Close()

	data := make(testResultV2)
	err = jsoniter.NewDecoder(fs).Decode(&data)
	if err != nil {
		return
	}

	setBestCdn(data)
}
