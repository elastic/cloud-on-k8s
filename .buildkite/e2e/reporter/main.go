package main

import (
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/joshdk/go-junit"
)

const (
	annotateSuccess  = "annotate-success"
	annotateFailures = "annotate-failures"
	notifyFailures   = "notify-failures"
)

var (
	xmlDir       string
	outputFormat string

	//go:embed templates/annotate-success.tpl.md
	annotationSuccessTpl string
	//go:embed templates/annotate-failures.tpl.md
	annotationFailuresTpl string
	//go:embed templates/notify-failures.tpl.yml
	notifyFailuresTpl string

	// extractSlugNameRe is a regexp to extract the name of the test environment from the cluster name
	// set by the pipeline generator via EnvVarClusterName.
	extractSlugNameRe = regexp.MustCompile("e2e-tests-eck-e2e-(.*)-[a-z]*-[0-9]*.xml")

	tplMap = map[string]string{
		annotateSuccess:  annotationSuccessTpl,
		annotateFailures: annotationFailuresTpl,
		notifyFailures:   notifyFailuresTpl,
	}
)

func init() {
	flag.StringVar(&xmlDir, "d", "./reports", "Directory containing JUnit XML reports to process")
	flag.StringVar(&outputFormat, "o", "", "Output format. One of: (annotate-success, annotate-failures, notify-failures)")
	flag.Parse()
}

func main() {
	tests := map[string]sortedTests{}
	failuresCount := 0

	// process all xml report in the given diretory
	err := filepath.Walk(xmlDir, func(xmlReportPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(xmlReportPath, ".xml") {
			// skip
			return nil
		}

		// extract slug name from the report file name
		match := extractSlugNameRe.FindStringSubmatch(xmlReportPath)
		if len(match) != 2 {
			return fmt.Errorf("failed to extract slug name from %s", xmlReportPath)
		}
		slugName := match[1]

		// ingest xml report to work with structs
		xmlReport, err := os.ReadFile(xmlReportPath)
		if err != nil {
			return err
		}
		// a suite groups all the tests of a package
		suites, err := junit.Ingest(xmlReport)
		if err != nil {
			return err
		}

		tests[slugName] = sortTests(suites)

		failuresCount += len(tests[slugName].Failed)

		return nil
	})
	if err != nil {
		exitWith(err)
	}

	srcTpl, ok := tplMap[outputFormat]
	if !ok {
		exitWith(fmt.Errorf("output format not supported"))
	}

	tpl, err := template.New("report").Funcs(template.FuncMap{
		"splitTestName": func(testName string) string {
			return strings.Split(testName, "/")[0]
		},
	}).Parse(srcTpl)
	if err != nil {
		exitWith(err)
	}
	err = tpl.Execute(os.Stdout, map[string]interface{}{
		"FailuresCount": failuresCount,
		"Tests":         tests,
	})
	if err != nil {
		exitWith(err)
	}

	if failuresCount > 0 {
		os.Exit(1)
	}
}

type sortedTests struct {
	Failed []junit.Test
	Passed []junit.Test
}

func sortTests(suites []junit.Suite) sortedTests {
	failedTests := []junit.Test{}
	passedTests := []junit.Test{}
	failedTestsMap := map[string]junit.Test{}

	// traverse all suites to find failed and passed tests
	for _, suite := range suites {
		for _, test := range suite.Tests {
			if test.Error != nil {
				// on test failure
				if strings.Contains(test.Name, "/") {
					// keep sub tests
					failedTests = append(failedTests, test)
				} else {
					// store parent tests for later
					failedTestsMap[test.Name] = test
				}
			} else {
				// on test success
				if !strings.Contains(test.Name, "/") {
					// ignore sub tests, keep parent tests
					passedTests = append(passedTests, test)
				}
			}
		}
	}

	// remove failed parent tests that have a failed sub test
	for _, subTest := range failedTests {
		testName := strings.Split(subTest.Name, "/")[0]
		delete(failedTestsMap, testName)
	}
	// add remaining failed parent tests
	for _, test := range failedTestsMap {
		failedTests = append(failedTests, test)
	}

	// add a failure if no failed or passed test was found
	if len(failedTests) == 0 && len(passedTests) == 0 {
		failedTests = []junit.Test{{Name: "NoTestRun", Error: errors.New("see job log")}}
	}

	return sortedTests{
		Failed: failedTests,
		Passed: passedTests,
	}
}

func exitWith(err error) {
	fmt.Printf("err: %v\n", err)
	os.Exit(1)
}
