package main

import (
	"io/ioutil"
	"log"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	gcfg "gopkg.in/gcfg.v1"
)

type Config struct {
	Search struct {
		CommandGrep       string
		SessionKeyGrep    string
		SessionCookieName string
	}
	Version struct {
		ConfigVersion string
	}
	Command map[string]*struct {
		ReqUrl           string
		ReqContentType   string
		ReqType          string
		ReqBody          string
		ReqUpload        string
		EncryptStartByte string
		EncryptNumBytes  string
		EncryptKey       string
		EncryptIv        string
		DoCall           string
		MsecDelay        string
		MsecRepeat       string
		Md5Input         string
		Base64Input      string
		MustCapture      string
		ReqHeaders       []string
		SessionVar       []string
	}
	CommandSequence struct {
		Sequence   string
		SessionLog string
	}
	FileName string
}

func NewConfig(configFile string) *Config {
	toReturn := &Config{FileName: configFile}
	bytes, err1 := ioutil.ReadFile(configFile)

	equalLine := regexp.MustCompile("=")
	openQuote := regexp.MustCompile("=[\t ]+")
	closeQuote := regexp.MustCompile("$") // (?m) is a flag that says to treat as multi-line strings (so that $ will match EOL instead of just EOF)
	output := ""
	str := strings.Split(string(bytes), "\n")
	for _, line := range str {
		if equalLine.MatchString(line) {
			line = strings.Replace(line, "\"", "\\\"", -1)
			line = openQuote.ReplaceAllString(line, "= \"")
			line = closeQuote.ReplaceAllString(line, "\"")
		}
		output += line + "\n"
	}
	if err1 != nil {
		log.Fatalf("Failed to open %s file. %s", configFile, err1)
	}
	err2 := gcfg.ReadStringInto(toReturn, output)
	if err2 != nil {
		log.Fatalf("Failed to parse gcfg data: %s", err2)
	}

	return toReturn
}

func (config *Config) FieldString(field string, command string) string {
	data := ""

	r := reflect.ValueOf(config.Command[command])
	if !reflect.Indirect(r).IsValid() {
		r = reflect.ValueOf(config.Command["default"])
	}
	if reflect.Indirect(r).IsValid() {
		f := reflect.Indirect(r).FieldByName(field)
		if f.String() != "" {
			data = f.String()
		} else {
			r := reflect.ValueOf(config.Command["default"])
			if reflect.Indirect(r).IsValid() {
				f := reflect.Indirect(r).FieldByName(field)
				data = f.String()
			}
		}
	}
	return data
}

func (config *Config) FieldInteger(field string, command string) int {
	str := config.FieldString(field, command)
	toReturn, err := strconv.Atoi(str)
	if err != nil {
		return 0
	}
	return toReturn
}

func (config *Config) MustCaptureElement(element string, command string) bool {
	var mustCapture bool = false
	str := config.FieldString("MustCapture", command)
	for _, elem := range strings.Fields(strings.Replace(str, ",", " ", -1)) {
		if elem == element {
			mustCapture = true
			break
		}
	}
	return mustCapture
}
