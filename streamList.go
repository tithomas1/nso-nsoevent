/*
Author:  Tim Thomas
Created: 21-Sep-2020
*/

package main

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

const (
	requestURLStreamList = "/data/ietf-restconf-monitoring:restconf-state/streams"
)

type StreamEncodingType int

const (
	ENCODING_UNKNOWN StreamEncodingType = iota
	ENCODING_JSON
	ENCODING_XML
)

func (s StreamEncodingType) String() string {
	return [...]string{"Unknown", "JSON", "XML"}[s]
}

/*
 Structure definitions based on what the top-level list of available streams looks like
 coming back from NSO.

 XML example:

<streams xmlns="urn:ietf:params:xml:ns:yang:ietf-restconf-monitoring" xmlns:rcmon="urn:ietf:params:xml:ns:yang:ietf-restconf-monitoring">
  <stream>
    <name>NETCONF</name>
    <description>default NETCONF event stream</description>
    <replay-support>false</replay-support>
    <access>
      <encoding>xml</encoding>
      <location>http://localhost:8080/restconf/streams/NETCONF/xml</location>
    </access>
    <access>
      <encoding>json</encoding>
      <location>http://localhost:8080/restconf/streams/NETCONF/json</location>
    </access>
  </stream>
</streams>
*/

/*
type IetfRestConfMonitoring struct {
	IetfRestConfStreams StreamList `json:"ietf-restconf-monitoring:streams"`
}
*/

type Access struct {
	XMLName      xml.Name           `xml:"access" json:"-"`
	EncodingType StreamEncodingType `xml:"-" json:"-"`
	Encoding     string             `xml:"encoding" json:"encoding"`
	LocationURL  *url.URL           `xml:"-" json:"-"`
	Location     string             `xml:"location" json:"location"`
}

type Stream struct {
	XMLName       xml.Name  `xml:"stream" json:"-"`
	Name          string    `xml:"name" json:"name"`
	Description   string    `xml:"description" json:"description"`
	ReplaySupport bool      `xml:"replay-support" json:"replay-support"`
	ReplayStart   time.Time `xml:"replay-log-creation-time" json:"replay-log-creation-time"`
	Access        []*Access `xml:"access" json:"access"`
	Webhooks      webhooks  `xml:"-" json:"-"`
}

type StreamList struct {
	XMLName xml.Name  `xml:"streams" json:"-"`
	Stream  []*Stream `xml:"stream" json:"stream"`
}

func newStreamList(rawData []byte) (*StreamList, error) {
	streamList := new(StreamList)

	err := xml.Unmarshal(rawData, &streamList)
	if err != nil {
		fmt.Printf("(newStreamList) xml.Unmarshal: %v", err)
		return nil, err
	}

	// Fill in extra fields and clean up

	for i, s := range streamList.Stream {
		for j, a := range s.Access {
			switch a.Encoding {
			case "json":
				streamList.Stream[i].Access[j].EncodingType = ENCODING_JSON
			case "xml":
				streamList.Stream[i].Access[j].EncodingType = ENCODING_XML
			default:
				streamList.Stream[i].Access[j].EncodingType = ENCODING_UNKNOWN
			}
			streamList.Stream[i].Access[j].LocationURL, err = url.Parse(fixupHostString(a.Location))
			if err != nil {
				return nil, err
				//panic(fmt.Errorf("(newStreamList) url.Parse failed on '%s': %v", a.Location, err))
			}
		}
	}

	return streamList, nil
}

func (streamList *StreamList) count() int {
	return len(streamList.Stream)
}

func (streamList *StreamList) findStreamsByName(targetName string) []*Stream {
	list := new([]*Stream)
	for i, s := range streamList.Stream {
		if fuzzyNameMatch(targetName, s.Name) {
			*list = append(*list, streamList.Stream[i])
		}
	}
	return *list
}

// Add a pointer back to a webhook

func (stream *Stream) addWebhook(webhook *webhook) {
	debugMsgf("(stream:addWebhook) [%s]\n",
		stringColorize(stream.Name, COLOR_STREAM))
	stream.Webhooks = append(stream.Webhooks, webhook)
}

func (streamList *StreamList) print() {
	/*
		for _, s := range streamList.Stream {
			fmt.Printf("stream name: %s (%s)\n", stringColorize(s.Name, COLOR_STREAM), s.Description)
			fmt.Printf("  encodings: ")
			for i, a := range s.Access {
				if i > 0 {
					fmt.Printf(",")
				}
				fmt.Printf(stringColorize(a.EncodingType.String(), COLOR_CYAN))
			}
			fmt.Printf(" / replay: %t\n", s.ReplaySupport)
		}
	*/

	count := len(streamList.Stream)
	if count == 0 {
		return
	}

	// Determine max column widths

	width := []int{0, 20, len("Replay"), 0}
	for _, item := range streamList.Stream {
		width = findMaxStringWidths(width, item.Name)
	}
	for i := 0; i < len(width)-1; i++ {
		width[i] = width[i] + outputColumnPadding
	}

	index := 0
	tablePrint(
		&width,
		&[]string{"Name", "Encodings", "Replay", "Description"},
		func() (*[]string, string) {
			if index >= count {
				return nil, ""
			}
			item := streamList.Stream[index]
			index++
			encodings := ""
			for i, a := range item.Access {
				if i > 0 {
					encodings = encodings + ","
				}
				encodings = encodings + a.EncodingType.String()
			}
			color := COLOR_CYAN
			if item.ReplaySupport {
				color = COLOR_YELLOW
			}
			columns := []string{
				stringColorize(item.Name, COLOR_STREAM),
				stringColorize(encodings, COLOR_CYAN),
				stringColorize(fmt.Sprintf("%v", item.ReplaySupport), color),
				stringColorize(stringCleanup(item.Description), COLOR_CYAN),
			}
			return &columns, ""
		})

	fmt.Printf("\n%s available stream%s\n", stringColorize(strconv.Itoa(count), COLOR_HI_YELLOW), pluralSuffix(count))
}
