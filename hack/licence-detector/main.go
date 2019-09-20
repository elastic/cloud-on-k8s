package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/build"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/elastic/cloud-on-k8s/hack/licence-detector/detector"
)

var (
	inFlag              = flag.String("in", "-", "Dependency list (output from go list -m -json all)")
	includeIndirectFlag = flag.Bool("includeIndirect", false, "Include indirect dependencies")
	outFlag             = flag.String("out", "-", "Path to output the notice information")
	templateFlag        = flag.String("template", "NOTICE.txt.tmpl", "Path to the template file")

	goModCache = filepath.Join(build.Default.GOPATH, "pkg", "mod")
)

func main() {
	flag.Parse()
	depInput, err := mkReader(*inFlag)
	if err != nil {
		log.Fatalf("Failed to create reader for %s: %v", *inFlag, err)
	}
	defer depInput.Close()

	dependencies, err := detector.Detect(depInput, *includeIndirectFlag)
	if err != nil {
		log.Fatalf("Failed to detect licences: %v", err)
	}

	if err := renderNotice(dependencies, *templateFlag, *outFlag); err != nil {
		log.Fatalf("Failed to render notice: %v", err)
	}
}

func mkReader(path string) (io.ReadCloser, error) {
	if path == "-" {
		return ioutil.NopCloser(os.Stdin), nil
	}

	return os.Open(path)
}

func renderNotice(dependencies *detector.Dependencies, templatePath, outputPath string) error {
	funcMap := template.FuncMap{
		"currentYear": CurrentYear,
		"line":        Line,
		"licenceText": LicenceText,
	}
	tmpl, err := template.New(filepath.Base(templatePath)).Funcs(funcMap).ParseFiles(templatePath)
	if err != nil {
		return fmt.Errorf("failed to parse template at %s: %w", templatePath, err)
	}

	w, cleanup, err := mkWriter(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file %s: %w", outputPath, err)
	}
	defer cleanup()

	if err := tmpl.Execute(w, dependencies); err != nil {
		return fmt.Errorf("failed to render template: %w", err)
	}

	return nil
}

func mkWriter(path string) (io.Writer, func(), error) {
	if path == "-" {
		return os.Stdout, func() {}, nil
	}

	f, err := os.Create(path)
	return f, func() { f.Close() }, err
}

/* Template functions */

func CurrentYear() string {
	return strconv.Itoa(time.Now().Year())
}

func Line(ch string) string {
	return strings.Repeat(ch, 80)
}

func LicenceText(licInfo detector.LicenceInfo) string {
	if licInfo.Error != nil {
		return licInfo.Error.Error()
	}

	var buf bytes.Buffer
	buf.WriteString("Contents of probable licence file ")
	buf.WriteString(strings.Replace(licInfo.LicenceFile, goModCache, "$GOMODCACHE", -1))
	buf.WriteString(":\n\n")

	f, err := os.Open(licInfo.LicenceFile)
	if err != nil {
		log.Fatalf("Failed to open licence file %s: %v", licInfo.LicenceFile, err)
	}
	defer f.Close()

	_, err = io.Copy(&buf, f)
	if err != nil {
		log.Fatalf("Failed to read licence file %s: %v", licInfo.LicenceFile, err)
	}

	return buf.String()
}
