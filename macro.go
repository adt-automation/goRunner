package main

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type UnixtimeMacro struct {
	duration time.Duration
	format   string
}

var PksInputs map[string]string = make(map[string]string)
var Md5Inputs map[string]string = make(map[string]string)
var Base64Inputs map[string]string = make(map[string]string)
var UnixtimeMacros map[string]UnixtimeMacro
var PrintTimeMacros map[string]UnixtimeMacro
var CommandMacros = make(map[string][]string)
var Md5Macros = make(map[string][]string)
var Base64Macros = make(map[string][]string)
var PksMacros = make(map[string][]string)

var reArgs = regexp.MustCompile("{%ARGS\\[(\\d+)\\]}")

func arrayContains(arr []string, str string) bool {
	i := 0
	for i < len(arr) {
		if str == arr[i] {
			break
		}
		i += 1
	}
	return i < len(arr)

}

func addCommandMacro(cmd string, macro string) {
	if !arrayContains(CommandMacros[cmd], macro) {
		CommandMacros[cmd] = append(CommandMacros[cmd], macro)
	}
}

func InitSessionLogMacros(sessionLog string) {
	// this will create an entry for a command named "\nSessionLog",
	// the newline will prevent any command named "SessionLog" in an ini file from overwriting it
	InitMacros("\nSessionLog", sessionLog)
}

func InitMacros(cmd string, field string) {
	_, exists := CommandMacros[cmd]
	if !exists {
		CommandMacros[cmd] = make([]string, 0)
	}

	rx, _ := regexp.Compile("\\{%.*?\\}")
	rxenv, _ := regexp.Compile("\\{\\$.*?\\}")

	for _, macro := range rx.FindAllString(field, -1) {
		addCommandMacro(cmd, macro)
	}
	for _, macro := range rxenv.FindAllString(field, -1) {
		addCommandMacro(cmd, macro)
	}
}

func parseTimeModifier(arg string) (time.Duration, error) {
	if len(arg) == 0 {
		return time.Duration(0), nil
	}

	rx := regexp.MustCompile("([\\+\\-]\\d+)(.*)")
	parsed := rx.FindStringSubmatch(arg)
	if parsed == nil {
		return time.Duration(0), errors.New(fmt.Sprintf("time modifier %s is not supported", arg))
	} else {
		mult, _ := strconv.Atoi(parsed[1]) // e.g -4, +4, 4
		if parsed[2] == "MONTH" || parsed[2] == "MONTHS" {
			return time.Duration(mult*30*24) * time.Hour, nil
		} else if parsed[2] == "DAY" || parsed[2] == "DAYS" {
			return time.Duration(mult) * 24 * time.Hour, nil
		} else if parsed[2] == "HOUR" || parsed[2] == "HOURS" {
			return time.Duration(mult) * time.Hour, nil
		} else if parsed[2] == "MINUTE" || parsed[2] == "MINUTES" {
			return time.Duration(mult) * time.Minute, nil
		} else if parsed[2] == "SECOND" || parsed[2] == "SECONDS" {
			return time.Duration(mult) * time.Second, nil
		} else {
			return time.Duration(0), errors.New(fmt.Sprintf("time modifier %s is not supported", arg))
		}
	}
}

func initUnixtimeMacro(macro string) {
	declaration := macro[2 : len(macro)-1]
	if strings.HasPrefix(declaration, "UNIXTIME") {
		arg := declaration[8:]
		rx1, _ := regexp.Compile("%(\\d+)?x")
		fmtmatch := rx1.FindString(arg)
		format := "%d"
		if len(fmtmatch) > 0 {
			format = fmtmatch
			arg = strings.Replace(arg, fmtmatch, "", -1)
		}
		duration, err := parseTimeModifier(arg)
		if err != nil {
			log.Fatalf("UNIXTIME macro %s: %s", declaration, err.Error())
		} else {
			UnixtimeMacros[macro] = UnixtimeMacro{duration, format}
		}
	}
}

func initPrintTimeMacro(macro string) {
	declaration := macro[2 : len(macro)-1]
	if strings.HasPrefix(declaration, "TIME") {
		format := "2006-01-02 15:04:05"
		arg := declaration[4:]
		duration, err := parseTimeModifier(arg)
		if err != nil {
			log.Fatalf("TIME macro %s: %s", declaration, err.Error())
		} else {
			PrintTimeMacros[macro] = UnixtimeMacro{duration, format}
		}
	}
}

func InitUnixtimeMacros() {
	PrintTimeMacros = make(map[string]UnixtimeMacro)
	UnixtimeMacros = make(map[string]UnixtimeMacro)
	for _, macros := range CommandMacros {
		for _, macro := range macros {
			initUnixtimeMacro(macro)
			initPrintTimeMacro(macro)
		}
	}
	for _, macros := range Base64Macros {
		for _, macro := range macros {
			initUnixtimeMacro(macro)
			initPrintTimeMacro(macro)
		}
	}
	for _, macros := range Md5Macros {
		for _, macro := range macros {
			initUnixtimeMacro(macro)
			initPrintTimeMacro(macro)
		}
	}
}

func addPksMacro(cmd string, macro string) {
	if !arrayContains(PksMacros[cmd], macro) {
		PksMacros[cmd] = append(PksMacros[cmd], macro)
	}
}

func InitPksMacro(cmd string, pksInput string) {
	if len(pksInput) == 0 {
		return
	}
	PksInputs[cmd] = pksInput
	PksMacros[cmd] = make([]string, 0)

	rx, _ := regexp.Compile("\\{%.*?\\}")
	rxenv, _ := regexp.Compile("\\{\\$.*?\\}")

	for _, macro := range rx.FindAllString(pksInput, -1) {
		addPksMacro(cmd, macro)
	}
	for _, macro := range rxenv.FindAllString(pksInput, -1) {
		addPksMacro(cmd, macro)
	}
} //InitPKSMacro
/*
func InitFuncMacro(cmd string, funcInput string) {
	if len(funcInput) == 0 {
		return
	}
	PksMacros[cmd] = make([]string, 0)

	rx, _ := regexp.Compile("\\{%.*?\\}")
	rxenv, _ := regexp.Compile("\\{\\$.*?\\}")

	for _, macro := range rx.FindAllString(funcInput, -1) {
		addPksMacro(cmd, macro)
	}
	for _, macro := range rxenv.FindAllString(funcInput, -1) {
		addPksMacro(cmd, macro)
	}
} //InitFuncMacro
*/
func addMd5Macro(cmd string, macro string) {
	if !arrayContains(Md5Macros[cmd], macro) {
		Md5Macros[cmd] = append(Md5Macros[cmd], macro)
	}
}

func InitMd5Macro(cmd string, md5Input string) {
	if len(md5Input) == 0 {
		return
	}

	Md5Inputs[cmd] = md5Input
	Md5Macros[cmd] = make([]string, 0)

	rx, _ := regexp.Compile("\\{%.*?\\}")
	rxenv, _ := regexp.Compile("\\{\\$.*?\\}")

	for _, macro := range rx.FindAllString(md5Input, -1) {
		addMd5Macro(cmd, macro)
	}
	for _, macro := range rxenv.FindAllString(md5Input, -1) {
		addMd5Macro(cmd, macro)
	}
}

func addBase64Macro(cmd string, macro string) {
	if !arrayContains(Base64Macros[cmd], macro) {
		Base64Macros[cmd] = append(Base64Macros[cmd], macro)
	}
}

func InitBase64Macro(cmd string, base64Input string) {
	if len(base64Input) == 0 {
		return
	}

	Base64Inputs[cmd] = base64Input
	Base64Macros[cmd] = make([]string, 0)

	rx, _ := regexp.Compile("\\{%.*?\\}")
	rxenv, _ := regexp.Compile("\\{\\$.*?\\}")

	for _, macro := range rx.FindAllString(base64Input, -1) {
		addBase64Macro(cmd, macro)
	}
	for _, macro := range rxenv.FindAllString(base64Input, -1) {
		addBase64Macro(cmd, macro)
	}
}

func _runnerMacro(command string, declaration string, inputData string, sessionVars map[string]string, reqTime time.Time) string {
	if !(strings.HasPrefix(declaration, "{%") || strings.HasPrefix(declaration, "{$")) || !strings.HasSuffix(declaration, "}") {
		return ""
	}
	//This func processes the token and returns the string
	uxt, ok := UnixtimeMacros[declaration]
	prt, ok1 := PrintTimeMacros[declaration]
	if ok {
		timestamp := reqTime.Add(uxt.duration).UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond)) //why not use now instead of reqTime?
		rx, _ := regexp.Compile("%(\\d+)x")
		fmtdigits := rx.FindStringSubmatch(uxt.format)
		if len(fmtdigits) == 0 {
			return fmt.Sprintf(uxt.format, timestamp)
		} else {
			fmtnum, _ := strconv.Atoi(fmtdigits[1])
			if fmtnum >= 12 {
				return fmt.Sprintf(uxt.format, timestamp)
			} else {
				tmp := fmt.Sprintf("%012x", timestamp)
				return tmp[0:fmtnum]
			}
		}
	} else if ok1 {
		return reqTime.Add(prt.duration).Format(prt.format)
	} else if declaration == "{%MD5SUM}" {
		testMd5 := Md5Inputs[command]
		for _, macro := range Md5Macros[command] {
			testMd5 = strings.Replace(testMd5, macro, runnerMacro(command, macro, inputData, sessionVars, reqTime), -1)
		}
		return strings.ToUpper(fmt.Sprintf("%x", md5.Sum([]byte(testMd5))))
	} else if declaration == "{%PKSENC}" {
		pksInput := PksInputs[command]
		for _, macro := range PksMacros[command] {
			pksInput = strings.Replace(pksInput, macro, runnerMacro(command, macro, inputData, sessionVars, reqTime), -1)
		}

		// that's dirty, waiting for proper func management
		inputs := strings.Split(pksInput, ",")
		if len(inputs) != 3 {
			return "invalid PKSInput format. Must be pwd,key,keyexp"
		}
		encryptor, err := NewPKSEncryptor(inputs[1], inputs[2], inputs[0])
		if err != nil {
			return err.Error()
		}
		res, err := encryptor.Encrypt()
		if err != nil {
			return err.Error()
		}
		return hex.EncodeToString(res)
	} else if declaration == "{%BASE64ENC}" {
		base64In := Base64Inputs[command]
		for _, macro := range Base64Macros[command] {
			base64In = strings.Replace(base64In, macro, runnerMacro(command, macro, inputData, sessionVars, reqTime), -1)
		}
		return base64.StdEncoding.EncodeToString([]byte(base64In))
	} else if strings.HasPrefix(declaration, "{$") {
		// an env var macro like {$SECRET}
		return os.Getenv(declaration[2 : len(declaration)-1])
	} else {
		// Check if it match {%ARGS[X]}
		argsIndex := reArgs.FindStringSubmatch(declaration) // regexp.MustCompile("{%ARGS\\[(\\d+)\\]}")
		if len(argsIndex) > 0 {
			i, _ := strconv.Atoi(argsIndex[1])
			arr := strings.Split(inputData, delimeter)
			if i >= len(arr) {
				// TODO : print error ?
				return ""
			}
			return arr[i]
		}
		// Check if it match a session var
		if declaration[1] == '%' {
			session_var := declaration[2 : len(declaration)-1]
			val, ok := sessionVars[session_var]
			if ok {
				return val
			}
		}
	}
	return ""
} //_runnerMacro replaces the variable reference like {%X} or {$X} with the value stored in the hash table

func runnerMacro(command string, declaration string, inputData string, sessionVars map[string]string, reqTime time.Time) string {
	if !(strings.HasPrefix(declaration, "{%") || strings.HasPrefix(declaration, "{$")) || !strings.HasSuffix(declaration, "}") {
		return ""
	} //This functions gets passed in a {%X} and {$X} variable reference, looks it up the hash table and returns the string value

	ssrx, _ := regexp.Compile("\\[(\\d+):(\\d+)\\]}")
	declSubstr := ssrx.FindStringSubmatch(declaration)
	if len(declSubstr) == 0 {
		return _runnerMacro(command, declaration, inputData, sessionVars, reqTime)
	} else {
		declaration = strings.Replace(declaration, declSubstr[0], "}", 1)
		result := _runnerMacro(command, declaration, inputData, sessionVars, reqTime)
		ss0, _ := strconv.Atoi(declSubstr[1])
		ss1, _ := strconv.Atoi(declSubstr[2])
		return result[ss0:ss1]
	}
}

func RunnerMacros(command string, inputData string, sessionVars map[string]string, reqTime time.Time, field string) string {
	for _, macro := range CommandMacros[command] {
		field = strings.Replace(field, macro, runnerMacro(command, macro, inputData, sessionVars, reqTime), -1)
	}
	return field
}

func RunnerMacrosRegexp(command string, inputData string, sessionVars map[string]string, reqTime time.Time, field string) string {
	for _, macro := range CommandMacros[command] {
		replacement := regexp.QuoteMeta(runnerMacro(command, macro, inputData, sessionVars, reqTime))
		field = strings.Replace(field, macro, replacement, -1)
	}
	return field
}

func SessionLogMacros(inputData string, sessionVars map[string]string, logTime time.Time, initial string) string {
	return RunnerMacros("\nSessionLog", inputData, sessionVars, logTime, initial)
}
