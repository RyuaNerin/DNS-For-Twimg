package main

import (
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"time"
)

const (
	defaultConfigPath = "./config.json"
)

type Config struct {
	Host					map[string]ConfigHost	`json:"host"`
	HTTP					ConfigHTTP				`json:"http"`
	DNS						ConfigDNS				`json:"dns"`
	Test					ConfigTest				`json:"test"`
	RPC						ConfigRPC				`json:"rpc"`
	LogLevel				int						`json:"log-level"`
	Path					ConfigPath				`json:"path"`
}
type ConfigHost struct {
	Host					string					`json:"host"`
	CDN						[]string				`json:"cdn"`
	Test					[]CnofigHostTest		`json:"test"`
}
type CnofigHostTest struct {
	URL						string					`json:"url"`
	SHA1					string					`json:"sha1"`
}
type ConfigHTTP struct {
	Type 					string					`json:"type"`
	Listen					string					`json:"listen"`
	TimeoutWrite			TimeDuration			`json:"timeout_write"`
	TimeoutRead				TimeDuration			`json:"timeout_read"`
	TimeoutIdle				TimeDuration			`json:"timeout_idle"`

	TemplatePath			string					`json:"template_path"`
	TemplateBufferSize		int						`json:"template_buffer"`

	WWWRoot					string					`json:"www-root"`

	NginxHeaderReadlIP		string					`json:"nginx-header-read-ip"`
}
type ConfigDNS struct {
	NameServer				[]string				`json:"server_nameserver"`
	ServerCacheExpire		TimeDuration			`json:"server_cache_expire"`
	ServerCacheMaxCount		int						`json:"server_cache_max_count"`

	DNSLookupTimeout		TimeDuration			`json:"dns_lookup_timeout"`
	DNSLookupInterval		TimeDuration			`json:"dns_lookup_interval"`

	OrganizationRegex		[]ConfigRegex			`json:"organization-pattern"`
	OrganizationIgnore		[]string				`json:"organization-ignore"`
}
type ConfigTest struct {
	RefreshInterval			TimeDuration			`json:"refresh_interval"`

	ThreatCrowdExpire		TimeDuration			`json:"threatcrowd_expire"`

	PingCount				int						`json:"ping_count"`
	PingTimeout				TimeDuration			`json:"ping_timeout"`

	HTTPCount				int						`json:"http_count"`
	HTTPTimeout				TimeDuration			`json:"http_timeout"`
	HTTPBufferSize			int						`json:"http_buffersize"`

	TwitterStatusTemplate	string					`json:"twitter-status-template"`
	TwitterAppKey			string					`json:"twitter-app-key"`
	TwitterAppSecret		string					`json:"twitter-app-secret"`
	TwitterUserKey			string					`json:"twitter-user-key"`
	TwitterUserSecret		string					`json:"twitter-user-secret"`
}
type ConfigRPC struct {
	Network					string					`json:"network"`
	Address					string					`json:"address"`
}
type ConfigPath struct {
	GeoIP2					string					`json:"geoip2"`
	TestSave				string					`json:"test-save"`
	StatSaveDNS				string					`json:"stat-save-dns"`
	StatSaveAPI				string					`json:"stat-save-api"`
	StatLog					string					`json:"stat-log"`
}

var config Config

func loadConfig(path string) {
	fs, err := os.Open(path)
	if err != nil {
		panic(err)
	}

	err = json.NewDecoder(fs).Decode(&config)
	if err != nil {
		panic(err)
	}
}

type TimeDuration struct {
	time.Duration
}
func (td *TimeDuration) MarshalJSON() ([]byte, error) {
	return json.Marshal(td.String())
}
func (td *TimeDuration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}

	switch value := v.(type) {
	case float64:
		td.Duration = time.Duration(value)
		return nil

	case string:
		var err error
		td.Duration, err = time.ParseDuration(value)
		if err != nil {
			return err
		}
		return nil

	default:
		return errors.New("invalid")
	}
}

type ConfigRegex struct {
	*regexp.Regexp
}

func (re *ConfigRegex) MarshalJSON() ([]byte, error) {
	return json.Marshal(re.Regexp.String())
}
func (re *ConfigRegex) UnmarshalJSON(b []byte) (err error) {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}

	switch value := v.(type) {
	case string:
		re.Regexp, err = regexp.Compile(value)
		if err != nil {
			return err
		}
		return nil

	default:
		return errors.New("invalid")
	}
}