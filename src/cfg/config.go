package cfg

import (
	"os"
	"time"
	"unsafe"

	"github.com/dustin/go-humanize"
	jsoniter "github.com/json-iterator/go"
)

const (
	configPath = "./config.json"
)

var V struct {
	HTTP struct {
		Server struct {
			ListenType string `json:"listen_type"`
			Listen     string `json:"listen"`
		}
		Client struct {
			Timeout struct {
				Timeout               time.Duration `json:"timeout"`
				IdleConnTimeout       time.Duration `json:"idle_conn_timeout"`
				ExpectContinueTimeout time.Duration `json:"expect_continue_timeout"`
				ResponseHeaderTimeout time.Duration `json:"response_header_timeout"`
				TLSHandshakeTimeout   time.Duration `json:"tls_handshake_timeout"`
			} `json:"timeout"`
		}
	}
	DNS struct {
		Client struct {
			LookupInterval time.Duration `json:"lookup_interval"`
			Timeout        struct {
				Timeout      time.Duration `json:"timeout"`
				ReadTimeout  time.Duration `json:"read_timeout"`
				WriteTimeout time.Duration `json:"write_timeout"`
				DialTimeout  time.Duration `json:"dial_timeout"`
			} `json:"client"`
		} `json:"client"`

		NameServerDefault []string            `json:"nameserver_default"`
		NameServer        map[string][]string `json:"nameserver"` // NameServer[Host]=[IP]
	} `json:"dns"`
	Test struct {
		RefreshInterval time.Duration `json:"refresh_interval"`

		ThreatCrowdExpire time.Duration `json:"threatcrowd_expire"`

		Worker struct {
			Resolve int `json:"resolve"`
			Ping    int `json:"ping"`
			Http    int `json:"http"`
		} `json:"worker"`

		PingCount   int           `json:"ping_count"`
		PingTimeout time.Duration `json:"ping_timeout"`

		HttpTimeout   time.Duration `json:"http_timeout"`
		HttpSpeedSize uint64        `json:"http_test_size"`

		Host map[string][]string `json:"host"` // 검사할 때 쓸 추가 호스트
	} `json:"test"`
	Path struct {
		ZoneFile string `json:"zone_file"`
		TestSave string `json:"test_save"`
		StatLog  string `json:"stat_log"`
	} `json:"path"`
}

func init() {
	jsoniter.RegisterTypeDecoderFunc(
		"uint64",
		func(ptr unsafe.Pointer, iter *jsoniter.Iterator) {
			switch iter.WhatIsNext() {
			case jsoniter.NilValue:
				iter.ReportError("uint64Decoder", "nil")

			case jsoniter.StringValue:
				v := iter.ReadString()
				i, err := humanize.ParseBytes(v)
				if err != nil {
					iter.ReportError("uint64Decoder", err.Error())
					return
				}
				*(*uint64)(ptr) = i

			case jsoniter.NumberValue:
				*(*uint64)(ptr) = uint64(iter.ReadUint64())

			default:
				iter.ReportError("uint64Decoder", "wrong type")
			}
		},
	)

	jsoniter.RegisterTypeDecoderFunc(
		"time.Duration",
		func(ptr unsafe.Pointer, iter *jsoniter.Iterator) {
			switch v := iter.Read().(type) {
			case string:
				td, err := time.ParseDuration(v)
				if err != nil {
					iter.Error = err
					return
				}
				*(*time.Duration)(ptr) = td

			case int:
				*(*time.Duration)(ptr) = time.Millisecond * time.Duration(v)

			case float64:
				*(*time.Duration)(ptr) = time.Duration(float64(time.Millisecond) * v)
			}
		},
	)
	jsoniter.RegisterTypeEncoderFunc(
		"time.Duration",
		func(ptr unsafe.Pointer, stream *jsoniter.Stream) {
			stream.WriteFloat64(float64(*(*time.Duration)(ptr)) / float64(time.Millisecond))
		},
		nil,
	)

	fs, err := os.Open(configPath)
	if err != nil {
		panic(err)
	}
	defer fs.Close()

	err = jsoniter.NewDecoder(fs).Decode(&V)
	if err != nil {
		panic(err)
	}
}
