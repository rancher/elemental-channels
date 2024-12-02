/*
Copyright © 2024 SUSE LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package parser

import (
	"bufio"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/davidcassany/updateinfo-parser/pkg/types"
)

const updateToken = "update"
const defaultTmpl = `{{define "join"}}--------------------------------------------------------------------------------
{{end}}
{{define "header"}}CHANGE LOG
{{template "join"}}{{end}}
{{define "body"}}{{.Title}}

ID: {{.ID}}
Type: {{.Type}}
Severity: {{.Severity}}
Date: {{.Issued.Date}}

Description:
{{.Description}}

{{if .References}}Issues:{{range .References}}
  * {{.Type}}: [{{.ID}}] {{.Title}}{{end}}

{{end}}{{end}}
{{define "footer"}}{{template "join"}}{{end}}`

type filterConfig struct {
	beforeDate   time.Time
	afterDate    time.Time
	dateFormat   string
	pkgWhiteList []string
	updateType   string
}

type outputConfig struct {
	output   io.Writer
	close    func() error
	template *template.Template
	jsonOut  bool
}

type FilterOpt func(*filterConfig) error

// WithDateFormat defines the format to parse date interavals according to time.Parse
// note this function must be provided before WithDateInterval to make it effective
func WithDateFormat(format string) FilterOpt {
	return func(f *filterConfig) error {
		f.dateFormat = format
		return nil
	}
}

// WithBeforeTime sets the filtering updates that happened before the given time
// if no format is set assumes a unix timestamp as string
func WithBeforeTime(before string) FilterOpt {
	return func(f *filterConfig) error {
		t, err := parseTime(before, f.dateFormat)
		if err != nil {
			return err
		}
		f.beforeDate = t
		return nil
	}
}

// WithAfterTime sets the filtering updates that happened after the given time
// if no format is set assumes a unix timestamp as string
func WithAfterTime(after string) FilterOpt {
	return func(f *filterConfig) error {
		t, err := parseTime(after, f.dateFormat)
		if err != nil {
			return err
		}
		f.afterDate = t
		return nil
	}
}

// WithPackagesFile sets the packages white list from a file, assumes OBS *.packages format
func WithPackagesFile(packagesFile string) FilterOpt {
	return func(f *filterConfig) error {
		var err error
		f.pkgWhiteList, err = readPackagesFile(packagesFile)
		if err != nil {
			return err
		}
		return nil
	}
}

// WithUpdateType defines the update type to select (e.g. security to only consider security updates)
func WithUpdateType(uType string) FilterOpt {
	return func(f *filterConfig) error {
		f.updateType = uType
		return nil
	}
}

func NewFilterConfig(opts ...FilterOpt) (*filterConfig, error) {
	fCfg := &filterConfig{
		beforeDate: time.Now().AddDate(100, 0, 0),
		afterDate:  time.Unix(0, 0),
	}
	for _, o := range opts {
		err := o(fCfg)
		if err != nil {
			return nil, err
		}
	}
	return fCfg, nil
}

type OutputOpt func(*outputConfig) error

func WithJsonOutput() OutputOpt {
	return func(o *outputConfig) error {
		o.jsonOut = true
		return nil
	}
}

func WithWriter(w io.Writer) OutputOpt {
	return func(o *outputConfig) error {
		o.output = w
		return nil
	}
}

func WithOutputFile(out string) OutputOpt {
	return func(o *outputConfig) error {
		f, err := os.Create(out)
		if err != nil {
			return err
		}
		o.output = f
		o.close = f.Close
		return nil
	}
}

func WithTemplate(t *template.Template) OutputOpt {
	return func(o *outputConfig) error {
		o.template = t
		return nil
	}
}

func WithTemplateFile(tmpl string) OutputOpt {
	return func(o *outputConfig) error {
		var err error
		o.template, err = template.ParseFiles(tmpl)
		return err
	}
}

func NewOutputConfig(opts ...OutputOpt) (*outputConfig, error) {
	oCfg := &outputConfig{
		output: os.Stdout,
	}
	for _, o := range opts {
		err := o(oCfg)
		if err != nil {
			return nil, err
		}
	}
	if oCfg.template != nil && oCfg.jsonOut {
		fmt.Fprintln(os.Stderr, "Warning: json output defined, ignoring provided template")
	} else if oCfg.template == nil && !oCfg.jsonOut {
		oCfg.template, _ = template.New("update").Parse(defaultTmpl)
	}
	return oCfg, nil
}

type UpdateHandlerFunc func(*types.Update) error

func Parse(reader io.Reader, filter filterConfig, handler UpdateHandlerFunc) error {
	d := xml.NewDecoder(reader)
	for {
		t, tokenErr := d.Token()
		if tokenErr != nil {
			if tokenErr == io.EOF {
				break
			}
			return fmt.Errorf("decoding token: %v", tokenErr)
		}
		switch t := t.(type) {
		case xml.StartElement:
			if t.Name.Local == updateToken {
				u := types.Update{}
				if err := d.DecodeElement(&u, &t); err != nil {
					return fmt.Errorf("decoding element %q: %v", t.Name.Local, err)
				}
				if filter.updateType != "" && u.Type != filter.updateType {
					continue
				}
				if u.Issued.Date == nil {
					continue
				}
				uDate := time.Time(*u.Issued.Date)
				if uDate.Before(filter.beforeDate) && uDate.After(filter.afterDate) {
					var pkgMatch bool
					for _, pkg := range u.Packages {
						if slices.Contains(filter.pkgWhiteList, pkg.Name) {
							pkgMatch = true
							break
						}
					}

					if len(filter.pkgWhiteList) == 0 || pkgMatch {
						err := handler(&u)
						if err != nil {
							return err
						}
					}
				}
			}
		}
	}

	return nil
}

func ParseToOutput(reader io.Reader, filter filterConfig, out outputConfig) (retErr error) {
	var err error
	if out.close != nil {
		defer func() {
			err = out.close()
			if retErr == nil && err != nil {
				retErr = err
			}
		}()
	}
	var handler UpdateHandlerFunc
	if !out.jsonOut {
		first := true
		err = out.template.ExecuteTemplate(out.output, "header", nil)
		if err != nil {
			return err
		}
		handler = func(u *types.Update) error {
			if !first {
				err = out.template.ExecuteTemplate(out.output, "join", nil)
				if err != nil {
					return err
				}
			}
			err = out.template.ExecuteTemplate(out.output, "body", &u)
			if err != nil {
				return err
			}
			first = false
			return nil
		}
		err = Parse(reader, filter, handler)
		if err != nil {
			return err
		}
		return out.template.ExecuteTemplate(out.output, "footer", nil)
	}
	updates := []*types.Update{}
	handler = func(u *types.Update) error {
		updates = append(updates, u)
		return nil
	}
	err = Parse(reader, filter, handler)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(updates, "", "  ")
	if err != nil {
		return err
	}

	_, err = out.output.Write(data)
	return err
}

func ParseFileToOutput(updateXML string, filter filterConfig, out outputConfig) (retErr error) {
	f, err := os.Open(updateXML)
	if err != nil {
		return nil
	}
	defer func() {
		err = f.Close()
		if retErr == nil && err != nil {
			retErr = err
		}
	}()

	return ParseToOutput(f, filter, out)
}

func readPackagesFile(pkgFile string) ([]string, error) {
	packages := []string{}

	if pkgFile == "" {
		return packages, nil
	}

	file, err := os.Open(pkgFile)
	if err != nil {
		return nil, fmt.Errorf("failed opening file '%s': %v", pkgFile, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// "|" is the separator of the csv *.packages file produced by OBS, first field is package name
		pkgData := strings.Split(strings.TrimSpace(line), "|")
		packages = append(packages, pkgData[0])
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed scanning lines on file '%s': %v", pkgFile, err)
	}
	return packages, nil
}

func parseTime(date, format string) (time.Time, error) {
	var err error
	var t time.Time

	if format == "" {
		var i int64
		i, err = strconv.ParseInt(date, 10, 64)
		if err != nil {
			return t, fmt.Errorf("failed parsing date '%s': %v", date, err)
		}
		t = time.Unix(i, 0)
		return t, nil
	}
	return time.Parse(format, date)
}
