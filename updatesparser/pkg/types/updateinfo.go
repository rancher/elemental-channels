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

package types

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

type UpdateInfo struct {
	XMLName xml.Name `xml:"updates"`
	Updates []Update `xml:"update"`
}

type Update struct {
	XMLName     xml.Name    `xml:"update" json:"-"`
	Type        string      `xml:"type,attr" json:"type,omitempty"`
	Status      string      `xml:"status,attr" json:"-"`
	ID          string      `xml:"id" json:"id,omitempty"`
	Title       string      `xml:"title" json:"title,omitempty"`
	Severity    string      `xml:"severity" json:"severity,omitempty"`
	Release     string      `xml:"release" json:"-"`
	Issued      Issued      `xml:"issued" json:"date,omitempty"`
	References  []Reference `xml:"references>reference" json:"references,omitempty"`
	Description string      `xml:"description" json:"description,omitempty"`
	Packages    []Package   `xml:"pkglist>collection>package" json:"packages,omitempty"`
}

type date time.Time

type Issued struct {
	XMLName xml.Name `xml:"issued" json:"-"`
	Date    *date    `xml:"date,attr"`
}

func (i Issued) MarshalJSON() ([]byte, error) {
	t := time.Time(*i.Date)
	return []byte(fmt.Sprintf("%q", strconv.FormatInt(t.Unix(), 10))), nil
}

func (d date) String() string {
	return time.Time(d).String()
}

func (d *date) UnmarshalXMLAttr(attr xml.Attr) error {
	ts, err := strconv.ParseInt(attr.Value, 10, 64)
	if err != nil {
		return err
	}
	t := time.Unix(ts, 0)
	*d = date(t)

	return nil
}

type href url.URL

type Reference struct {
	XMLName xml.Name `xml:"reference" json:"-"`
	URL     href     `xml:"href,attr" json:"href,omitempty"`
	ID      string   `xml:"id,attr" json:"id,omitempty"`
	Title   string   `xml:"title,attr" json:"title,omitempty"`
	Type    string   `xml:"type,attr" json:"type,omitempty"`
}

func (h href) String() string {
	u := url.URL(h)
	return u.String()
}

func (h *href) UnmarshalXMLAttr(attr xml.Attr) error {
	u, err := url.Parse(attr.Value)
	if err != nil {
		return err
	}
	*h = href(*u)
	return nil
}

func (h *href) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%q", h.String())), nil
}

type Package struct {
	XMLName  xml.Name `xml:"package" json:"-"`
	Name     string   `xml:"name,attr" json:"name,omitempty"`
	Version  string   `xml:"version,attr" json:"version,omitempty"`
	Release  string   `xml:"release,attr" json:"release,omitempty"`
	Arch     string   `xml:"arch,attr" json:"arch,omitempty"`
	Filename string   `xml:"filename" json:"-"`
}
