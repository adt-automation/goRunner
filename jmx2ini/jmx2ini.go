package main

import (
	"fmt"
	"launchpad.net/xmlpath"
	"os"
	"sort"
	"strings"
)

var xpathRoot *xmlpath.Node

func getValue(xpath string, node *xmlpath.Node, defaultValue string) string {
	path, err := xmlpath.Compile(xpath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%x error: %v\n", xpath, err)
		return defaultValue
	}
	if value, ok := path.String(node); ok {
		return value
	} else {
		return defaultValue
	}
}

func printValue(xpath string, optionalRoot ...*xmlpath.Node) {
	root := xpathRoot
	if len(optionalRoot) > 0 {
		root = optionalRoot[0]
	}
	value := getValue(xpath, root, "not found")
	fmt.Println(xpath, ": ", value)
}

func getBooleanValue(xpath string, node *xmlpath.Node, defaultValue bool) bool {
	tf := getValue(xpath, node, "default")
	if tf == "false" {
		return false
	} else if tf == "true" {
		return true
	} else {
		return defaultValue
	}
}

func getNodes(xpath string, optionalRoot ...*xmlpath.Node) *xmlpath.Iter {
	path, err := xmlpath.Compile(xpath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%x error: %v\n", xpath, err)
		var i *xmlpath.Iter
		return i
	}
	root := xpathRoot
	if len(optionalRoot) > 0 {
		root = optionalRoot[0]
	}
	return path.Iter(root)
}

func walkNode(node *xmlpath.Node) {
	if !getBooleanValue("@enabled", node, false) {
		return
	}
	printValue("@testclass", node)
	nodeHash := getNodes("following-sibling::hashTree", node)
	if nodeHash.Next() {
		children := getNodes("node()[@testclass]", nodeHash.Node())
		for children.Next() {
			walkNode(children.Node())
		}
	}
}

func HeaderManager(node *xmlpath.Node) map[string]string {
	// HeaderManager cannot have children (sibling hashTree will alway be empty)
	headers := make(map[string]string)
	headerNodes := getNodes("//elementProp[@elementType='Header']", node)
	for headerNodes.Next() {
		header := getValue("stringProp[@name='Header.name']", headerNodes.Node(), "")
		if len(header) > 0 {
			value := getValue("stringProp[@name='Header.value']", headerNodes.Node(), "")
			headers[header] = value
		}
	}
	return headers
}

func Arguments(node *xmlpath.Node) map[string]string {
	// Arguments cannot have children (sibling hashTree will alway be empty)
	args := make(map[string]string)
	argNodes := getNodes("collectionProp[@name='Arguments.arguments']/elementProp[@elementType='Argument']", node)
	for argNodes.Next() {
		arg := getValue("stringProp[@name='Argument.name']", argNodes.Node(), "")
		if len(arg) > 0 {
			arg = fmt.Sprintf("${%s}", arg)
			value := getValue("stringProp[@name='Argument.value']", argNodes.Node(), "")
			args[arg] = value
		}
	}
	return args
}

func ConfigTestElement(node *xmlpath.Node) (map[string]string, map[string]string) {
	// ConfigTestElement cannot have children (sibling hashTree will alway be empty)
	props := make(map[string]string)
	propNodes := getNodes("stringProp", node)
	for propNodes.Next() {
		prop := getValue("@name", propNodes.Node(), "")
		if len(prop) > 0 {
			props[prop] = getValue("node()", propNodes.Node(), "")
		}
	}
	return props, Arguments(node)
}

func HTTPSamplerProxy(arguments map[string]string, httpParams map[string]string, node *xmlpath.Node) (string, string) {

	configEntries := make([]string, 0)

	httpConfig := make(map[string]string)
	propNodes := getNodes("stringProp", node)
	for propNodes.Next() {
		prop := getValue("@name", propNodes.Node(), "")
		value := getValue("node()", propNodes.Node(), "")
		if len(prop) > 0 && len(value) > 0 {
			httpConfig[prop] = value
		}
	}
	propNodes = getNodes("boolProp", node)
	for propNodes.Next() {
		prop := getValue("@name", propNodes.Node(), "")
		value := getValue("node()", propNodes.Node(), "")
		if len(prop) > 0 && len(value) > 0 {
			httpConfig[prop] = value
		}
	}
	//fmt.Println("httpConfig\n", httpConfig)
	if len(httpConfig["HTTPSampler.method"]) > 0 {
		configEntries = append(configEntries, fmt.Sprintf("ReqType = %s", httpConfig["HTTPSampler.method"]))
	}

	reqUrl := httpConfig["HTTPSampler.path"]
	params := make(map[string]string)
	for param, value := range httpParams {
		params[param] = value
	}
	paramNodes := getNodes("elementProp/collectionProp/elementProp", node)
	for paramNodes.Next() {
		param := getValue("stringProp[@name='Argument.name']", paramNodes.Node(), "")
		if len(param) > 0 {
			value := getValue("stringProp[@name='Argument.value']", paramNodes.Node(), "")
			params[param] = value
		}
	}
	queryParams := make([]string, 0)
	for param, value := range params {
		queryParams = append(queryParams, fmt.Sprintf("%s=%s", param, value)) // TODO already-encoded flag
	}
	if len(queryParams) > 0 {
		reqUrl = reqUrl + "?" + strings.Join(queryParams, "&")
	}
	configEntries = append(configEntries, fmt.Sprintf("ReqUrl = %s", reqUrl))

	testname := getValue("@testname", node, "")
	return testname, strings.Join(configEntries, "\n")
}

func ConstantTimerDelay(node *xmlpath.Node) string {
	delay := ""
	propNodes := getNodes("stringProp[@name='ConstantTimer.delay']", node)
	if propNodes.Next() {
		delay = getValue("node()", propNodes.Node(), "")
	}
	return delay
}

func LoopController(headers map[string]string, arguments map[string]string, httpConfig map[string]string, httpParams map[string]string, node *xmlpath.Node) {

	fmt.Println("; goRunner ini file generated by jmx2ini")

	loopCount := getValue("stringProp[@name='LoopController.loops']", node, "100")
	baseUrl, ok := httpConfig["HTTPSampler.protocol"]
	if !ok || len(baseUrl) == 0 {
		baseUrl = "http"
	}
	baseUrl = baseUrl + "://" + httpConfig["HTTPSampler.domain"]
	httpPort, ok := httpConfig["HTTPSampler.port"]
	if ok {
		baseUrl = baseUrl + ":" + httpPort
	}
	fmt.Printf("; seq %s | goRunner -baseUrl=%s -configFile=filename.ini -c 1\n", loopCount, baseUrl)

	commandSequence := make([]string, 0)
	commands := make(map[string]string)
	msecDelay := ""

	nodeHash := getNodes("following-sibling::hashTree", node)
	if nodeHash.Next() {
		children := getNodes("node()[@testclass]", nodeHash.Node())
		for children.Next() {
			testClass := getValue("@testclass", children.Node(), "")
			enabled := getBooleanValue("@enabled", children.Node(), false)
			if testClass == "HTTPSamplerProxy" && enabled {
				testname, testattrs := HTTPSamplerProxy(arguments, httpParams, children.Node())
				commandSequence = append(commandSequence, testname)
				commands[testname] = testattrs
			} else if testClass == "ConstantTimer" {
				msecDelay = ConstantTimerDelay(children.Node())
			}
		}
	}

	fmt.Println()
	fmt.Println("[commandSequence]")
	fmt.Println("Sequence = ", strings.Join(commandSequence, ", "))
	fmt.Println()
	fmt.Println("[command \"default\"]")
	// sort headers so they appear in the same order every time, easier to diff results this way
	sortedHeaders := make([]string, 0)
	for header, value := range headers {
		sortedHeaders = append(sortedHeaders, fmt.Sprintf("ReqHeaders = %s: %s", header, value))
	}
	sort.Strings(sortedHeaders)
	for _, header := range sortedHeaders {
		fmt.Println(header)
	}
	fmt.Printf("MSecDelay = %s\n", msecDelay)
	for _, command := range commandSequence {
		fmt.Println()
		fmt.Printf("[command \"%s\"]\n", command)
		fmt.Println(commands[command])
	}
}

func TransactionController(headers map[string]string, arguments map[string]string, httpConfig map[string]string, httpParams map[string]string, node *xmlpath.Node) {
	nodeHash := getNodes("following-sibling::hashTree", node)
	if nodeHash.Next() {
		children := getNodes("node()[@testclass]", nodeHash.Node())
		for children.Next() {
			testClass := getValue("@testclass", children.Node(), "")
			if testClass == "LoopController" {
				LoopController(headers, arguments, httpConfig, httpParams, children.Node())
			}
		}
	}
}

func walkThreadGroup(node *xmlpath.Node) {
	var headers = make(map[string]string)
	var arguments = make(map[string]string)
	var httpConfig = make(map[string]string)
	var httpParams = make(map[string]string)

	// TODO the ThreadGroup has some config as its direct children too

	nodeHash := getNodes("following-sibling::hashTree", node)
	if nodeHash.Next() {
		children := getNodes("node()[@testclass]", nodeHash.Node())
		for children.Next() {
			testClass := getValue("@testclass", children.Node(), "")
			if testClass == "HeaderManager" {
				headers = HeaderManager(children.Node())
			} else if testClass == "Arguments" {
				arguments = Arguments(children.Node())
			} else if testClass == "ConfigTestElement" {
				httpConfig, httpParams = ConfigTestElement(children.Node())
			} else if testClass == "TransactionController" {
				for k, v := range httpConfig {
					a, found := arguments[v]
					if found {
						httpConfig[k] = a
					}
				}
				TransactionController(headers, arguments, httpConfig, httpParams, children.Node())
			}
		}
	}
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: jmx2ini input-file.jmx >output-file.ini\n")
		os.Exit(1)
	}
	file, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	xpathRoot, err = xmlpath.Parse(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "xmlpath.Parse error: %v\n", err)
		os.Exit(1)
	}

	testPlans := getNodes("//TestPlan[@enabled='true']")
	for testPlans.Next() {
		children := getNodes("following-sibling::hashTree", testPlans.Node())
		if children.Next() {
			threadGroups := getNodes("ThreadGroup[@enabled='true']", children.Node())
			for threadGroups.Next() {
				walkThreadGroup(threadGroups.Node())
			}
		}
	}
}
