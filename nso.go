/*
Author:  Tim Thomas
Created: 23-Sep-2020
*/

package main

import (
	"context"
	"crypto/tls"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	requestURLRootResource = "/.well-known/host-meta"
)

type ResponseContentType int

const (
	ResponseContentUnknown ResponseContentType = iota
	ResponseContentHTML
	ResponseContentJSON
	ResponseContentXML
)

func (t ResponseContentType) String() string {
	return [...]string{"Unknown", "HTML", "JSON", "XML"}[t]
}

type NsoServer struct {
	apiUrl       string
	apiPort      int
	user         string
	password     string
	RootResource string
	Version      string
	State        *State
	StreamList   *StreamList
}

func newNSOServer() *NsoServer {
	return &NsoServer{
		apiUrl:   Config.nsoTarget.apiUrl,
		apiPort:  Config.nsoTarget.port,
		user:     Config.nsoTarget.user,
		password: Config.nsoTarget.password,
	}
}

type Link struct {
	XMLName xml.Name `xml:"Link"`
	Rel     string   `xml:"rel,attr"`
	Href    string   `xml:"href,attr"`
}

type Xrd struct {
	XMLName xml.Name `xml:"XRD"`
	Link    []Link   `xml:"Link"`
}

// Get the root resource URL from the server. It's the basis for all other API calls.
// It's likely "/restconf", but it's possible to change it in ncs.conf

func (s *NsoServer) getRootResource() error {

	// A BAD idea, but for now...
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	data, contentType, err := s.getResourceData(requestURLRootResource)
	if err != nil {
		return err
	}

	if contentType != ResponseContentXML {
		return fmt.Errorf("(NSOServer:getRootResource) expected XML response, got %s - may be an authentication error", contentType.String())
	}

	xrd := new(Xrd)
	err = xml.Unmarshal(data, &xrd)
	if err != nil {
		fmt.Printf("(NSOServer:getRootResource) xml.Unmarshal: %v", err)
		return err
	}

	if href := xrd.findHref("restconf"); href == "" {
		return errors.New("(NSOServer:getRootResource) unable to determine HREF for 'restconf'")
	} else {
		s.RootResource = href
	}

	return nil
}

// Retrieve the server's list of available event streams

func (s *NsoServer) getStreamList() error {
	data, _, err := s.getResourceData(requestURLStreamList)
	if err != nil {
		return err
	}

	streamList, err := newStreamList(data)
	if err != nil {
		fmt.Printf("(getStreamList) newStreamList: %v", err)
		return err
	}

	s.StreamList = streamList
	return nil
}

func (s *NsoServer) findStreamsByName(targetName string) []*Stream {
	return s.StreamList.findStreamsByName(targetName)
}

// Get the raw data associated with a particular resource URL

func (s *NsoServer) getResourceData(r string) ([]byte, ResponseContentType, error) {
	resourceUrl := s.RootResource + r
	reqUrl, err := url.Parse(s.apiUrl + resourceUrl)
	if err != nil {
		return nil, ResponseContentUnknown, err
	}

	req := &http.Request{
		Method: "GET",
		URL:    reqUrl,
		Header: map[string][]string{
			"Accept": {"application/yang-data+xml"},
		},
		Body: nil,
	}

	// Basic authentication header
	req.SetBasicAuth(s.user, s.password)

	// Issue the request with specified timeout
	ctx, cancel := context.WithTimeout(context.Background(), Config.readTimeout)
	defer cancel()

	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		if e, ok := err.(net.Error); ok && e.Timeout() {
			fmt.Println("(NSOServer:getResourceData) " + stringColorize("URL timeout", COLOR_ERROR))
		}
		return nil, ResponseContentUnknown, err
	}

	debugMsgf("(NSOServer:getResourceData) HTTP %s response for %s\n",
		stringColorize(strconv.Itoa(resp.StatusCode), COLOR_HI_BLUE),
		stringColorize(req.URL.String(), COLOR_URL))

	data, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	contentType := determineResponseType(resp.Header["Content-Type"])

	if Config.verbose {
		//fmt.Printf("(NSOServer:getResourceData) response headers [%s]:\n%q\n", resourceUrl, resp.Header)
		fmt.Printf("(NSOServer:getResourceData) response headers [%s]:\n%q\n", reqUrl, resp.Header)
		fmt.Printf("(NSOServer:getResourceData) %s response body:\n%s\n",
			stringColorize(contentType.String(), COLOR_HI_BLUE), string(data))
	}

	return data, contentType, nil
}

// Open a persistent event stream, returning an io.ReadCloser

func (s *NsoServer) openStream(reqUrl *url.URL) (io.ReadCloser, error) {
	req := &http.Request{
		Method: "GET",
		URL:    reqUrl,
		Header: map[string][]string{
			"Accept":        {"text/event-stream"},
			"Cache-Control": {"no-cache"},
			"Connection":    {"keep-alive"},
		},
		Body: nil,
	}

	// Basic authentication header
	req.SetBasicAuth(s.user, s.password)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

// Validate list of webhooks against the server's list of available streams

func (s *NsoServer) validateWebhooks() {
	Config.webhooks.validate(s.StreamList)
}

// Find an href using a rel

func (x *Xrd) findHref(rel string) string {
	for _, l := range x.Link {
		if l.Rel == rel {
			return l.Href
		}
	}
	return ""
}

// TODO Use http.DetectContentType() instead? It guesses XML as text/plain
func determineResponseType(contentType []string) ResponseContentType {
	for _, t := range contentType {
		if strings.Contains(t, "text/html") {
			return ResponseContentHTML
		}
		if strings.Contains(t, "xml") {
			return ResponseContentXML
		}
		if strings.Contains(t, "json") {
			return ResponseContentJSON
		}
	}
	return ResponseContentUnknown
}

func (s *NsoServer) printState() {
	fmt.Printf("NSO server %s\n", stringColorize(s.apiUrl, COLOR_HI_YELLOW))
	fmt.Printf("RESTCONF root resource = %s\n", stringColorize(s.RootResource, COLOR_HI_YELLOW))
	fmt.Printf("NSO version %s\n", stringColorize(s.Version, COLOR_HI_YELLOW))
	streamCount := s.streamCount()
	fmt.Printf("%s available stream%s\n", stringColorize(strconv.Itoa(streamCount), COLOR_HI_YELLOW), pluralSuffix(streamCount))
	modelCount := s.dataModelCount()
	fmt.Printf("%s loaded data model%s", stringColorize(strconv.Itoa(modelCount), COLOR_HI_YELLOW), pluralSuffix(modelCount))
	if mountCount := s.mountCount(); mountCount > 0 {
		fmt.Printf(" with %s mount%s\n", stringColorize(strconv.Itoa(mountCount), COLOR_HI_YELLOW), pluralSuffix(mountCount))
	} else {
		fmt.Println()
	}
	callpointCount := s.callpointCount()
	fmt.Printf("%s callpoint%s", stringColorize(strconv.Itoa(callpointCount), COLOR_HI_YELLOW), pluralSuffix(callpointCount))
	if actionpointCount := s.actionpointCount(); actionpointCount > 0 {
		fmt.Printf(" with %s actionpoint%s\n", stringColorize(strconv.Itoa(actionpointCount), COLOR_HI_YELLOW), pluralSuffix(actionpointCount))
	} else {
		fmt.Println()
	}
	datastoreCount := s.datastoreCount()
	fmt.Printf("%s datastore%s\n", stringColorize(strconv.Itoa(datastoreCount), COLOR_HI_YELLOW), pluralSuffix(datastoreCount))
}

func (s *NsoServer) printModels() {
	s.printModelList()
}

// Simple function to print the raw output from a particular NSO API URL

func (s *NsoServer) printAPIData(urlString string) error {
	data, _, err := s.getResourceData(urlString)
	if err != nil {
		return err
	}
	fmt.Printf("%s %s [%d bytes]\n",
		stringColorize("API data for:", COLOR_HEADINGS),
		stringColorize(urlString, COLOR_URL), len(data))
	fmt.Printf("%s\n", string(data))
	return nil
}
