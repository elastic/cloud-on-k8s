package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/joshdk/go-junit"
)

var (
	// extractSlugNameRe is a regexp to extract the name of the test environment.
	// It is set by the pipeline generator via EnvVarClusterName.
	extractSlugNameRe = regexp.MustCompile("e2e-tests-eck-e2e-(.*)-[a-z]*-[0-9]*.xml")
)

func main() {
	if len(os.Args) != 2 {
		exitWith(errors.New("argument 'directory' required"))
	}

	failures := 0

	// process all xml report in the given diretory
	err := filepath.Walk(os.Args[1], func(xmlReportPath string, info os.FileInfo, err error) error {
		if err != nil {
			exitWith(err)
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(xmlReportPath, ".xml") {
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

		failedTests, passedTests := sortTests(suites)

		failures += printFinalReport(slugName, failedTests, passedTests)

		return nil
	})
	if err != nil {
		exitWith(err)
	}

	if failures > 0 {
		os.Exit(1)
	}
}

func sortTests(suites []junit.Suite) ([]junit.Test, []junit.Test) {
	failedTests := []junit.Test{}
	passedTests := []junit.Test{}
	failedTestsMap := map[string]junit.Test{}

	// traverse all suites to find failed and passed tests
	for _, suite := range suites {
		if len(suite.Tests) == 0 {
			continue
		}
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

	return failedTests, passedTests
}

func printFinalReport(envName string, failedTests, passedTests []junit.Test) int {
	// success
	if len(failedTests) == 0 && len(passedTests) > 0 {
		return 0
	}

	// fail if no failed or passed test was found
	if (len(failedTests) + len(passedTests)) == 0 {
		fmt.Println("<details>")
		fmt.Printf("<summary>🐞 <code>%s</code> ~ %s</summary>\n", "NoTestRun", envName)
		fmt.Printf("\n```\n%s\n```\n", "See job log.")
		fmt.Println("</details>")
		fmt.Println("<br>")
		return 1
	}

	// failures
	for _, test := range failedTests {
		fmt.Println("<details>")
		fmt.Printf("<summary>🐞 <code>%s</code> ~ %s</summary>\n", test.Name, envName)
		fmt.Printf("\n```\n%s```\n", test.Error.Error())
		fmt.Println("</details>")
		fmt.Println("<br>")
	}
	return len(failedTests)
}

func exitWith(err error) {
	fmt.Printf("err: %v\n", err)
	os.Exit(1)
}
