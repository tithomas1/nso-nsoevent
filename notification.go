/*
Author:  Tim Thomas
Created: 25-Sep-2020
*/

package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"regexp"
	"sort"
	"time"
)

/*
The high-level notification format is defined in RFC 7950, section 4.2.10

The only top-level field is eventTime. Any subsequent tags are dependent on the
notification definition.
*/

type EventType int

const (
	EVENT_UNKNOWN EventType = iota
	EVENT_NCS_COMMIT_QUEUE_PROGRESS
	EVENT_NETCONF_SESSION_START
	EVENT_NETCONF_CONFIG_CHANGE
)

func (e EventType) String() string {
	return [...]string{
		"unknown",
		"ncs-commit-queue-progress",
		"netconf-session-start",
		"netconf-config-change",
	}[e]
}

// Main event notification structure. Some of these fields may/may not be filled in
// depending on the specific event that occurred

type Notification struct {
	XMLName     xml.Name                              `xml:"notification" json:"-"`
	EventTime   time.Time                             `xml:"eventTime" json:"eventTime"`
	EventName   string                                `xml:"-"`
	EventType   EventType                             `xml:"-"`
	User        string                                `xml:"-"`
	UserHost    string                                `xml:"-"`
	Datastore   string                                `xml:"-"`
	Devices     []string                              `xml:"-"`
	Edits       []*NetconfConfigChangeEdit            `xml:"-"`
	DeviceEdits map[string][]*NetconfConfigChangeEdit `xml:"-"`
	Inner       []byte                                `xml:",innerxml"`
}

// The initial sizing in the new Notification is somewhat arbitrary

func newNotification() *Notification {
	return &Notification{
		EventType:   EVENT_UNKNOWN,
		EventName:   EVENT_UNKNOWN.String(),
		DeviceEdits: make(map[string][]*NetconfConfigChangeEdit, 10),
	}
}

func (n *Notification) setEventType(e EventType) {
	n.EventType = e
	n.EventName = n.EventType.String()
}

// Event structures that may be present. The event-specific structures will get filled in
// by xml.Unmarshal() if the corresponding node is present

type NotificationInner struct {
	//XMLName                xml.Name                `xml:"notification" json:"-"`
	EventTime              time.Time               `xml:"eventTime" json:"eventTime"`
	NcsCommitQueueProgress *NcsCommitQueueProgress `xml:"ncs-commit-queue-progress-event" json:"-"`
	NetconfSessionStart    *NetconfSessionStart    `xml:"netconf-session-start" json:"-"`
	NetconfConfigChange    *NetconfConfigChange    `xml:"netconf-config-change" json:"-"`
}

var reEventName = regexp.MustCompile("</eventTime>[\\s]*<([^\\s^>]+)")

func (n *Notification) handlerDefault(_ streamSubscriber) (string, error) {
	innerClean := string(xmlInnerCleanup(n.Inner))

	// Try to determine the event name/type from the first node after eventTime

	if subMatches := reEventName.FindStringSubmatch(innerClean); subMatches != nil {
		n.EventName = subMatches[1]
	}

	return fmt.Sprintf("inner structure:\n%s\n", innerClean), nil
}

//**********
// ncs-event stream event handler
//**********
//
// From nso:tailf-ncs-alarms.yang
// From nso:tailf-kicker.yang
// From nso:tailf-ncs-plan.yang.yang
// From nso:tailf-ncs-devices.yang

type NcsCommitQueueProgress struct {
	//XMLName           xml.Name             `xml:"ncs-commit-queue-progress-event" json:"-"`
	Id                uint64               `xml:"id" json:"-"`
	Tag               string               `xml:"tag" json:"-"`
	State             string               `xml:"state" json:"-"`
	CompletedServices []*CompletedServices `xml:"completed-services" json:"-"`
	FailedServices    []*FailedServices    `xml:"failed-services" json:"-"`
	CompletedDevices  []*CompletedDevices  `xml:"completed-devices" json:"-"`
	TransientDevices  []*TransientDevices  `xml:"transient-devices" json:"-"`
	FailedDevices     []*FailedDevices     `xml:"failed-devices" json:"-"`
}

type CompletedServices struct {
	//XMLName          xml.Name            `xml:"completed-services" json:"-"`
	Name             string              `xml:"name" json:"-"`
	CompletedDevices []*CompletedDevices `xml:"completed-devices" json:"-"`
}

type FailedServices struct {
	//XMLName          xml.Name            `xml:"failed-services" json:"-"`
	Name             string              `xml:"name" json:"-"`
	CompletedDevices []*CompletedDevices `xml:"completed-devices" json:"-"`
}

// TODO: These two structures could be collapsed into one

type CompletedDevices struct {
	//XMLName xml.Name `xml:"completed-devices" json:"-"`
	Name string `xml:"name" json:"-"`
}

type TransientDevices struct {
	//XMLName xml.Name `xml:"transient-devices" json:"-"`
	Name string `xml:"name" json:"-"`
}

type FailedDevices struct {
	//XMLName xml.Name `xml:"failed-devices" json:"-"`
	Name   string `xml:"name" json:"-"`
	Reason string `xml:"reason" json:"-"`
}

func (n *Notification) handlerNcsEvents(sub streamSubscriber) (string, error) {
	nInner := new(NotificationInner)

	// Fake a root node so unmarshal is happy. I guess the notification could all be decoded
	// before this point into the outer Notification structure, but does it make sense to
	// decode the rest into the main structure here? The main structure already has a root
	// node...

	// TODO: Do there really need to be 2 Notification structures?

	// debugMsgf("[%s] (Notification:handlerNcsEvents) inner:\n%s\n",
	//  	stringColorize(sub.stream.Name, COLOR_STREAM), xmlInnerCleanup(n.Inner))

	rootInner := append(append([]byte("<root>"), n.Inner[:]...), []byte("</root>")...)
	err := xml.Unmarshal(rootInner, &nInner)
	if err != nil {
		return "", err
	}

	// Only handle commit-queue-progress for now

	if nInner.NcsCommitQueueProgress == nil {
		return fmt.Sprintf("(handler) no known event data found!"), nil
	}

	n.setEventType(EVENT_NCS_COMMIT_QUEUE_PROGRESS)
	msg := fmt.Sprintf("[%s]", stringColorize(n.EventType.String(), COLOR_EVENT))
	msg = msg + fmt.Sprintf(" [id %d]", nInner.NcsCommitQueueProgress.Id)
	msg = msg + fmt.Sprintf(" %s - %s",
		nInner.NcsCommitQueueProgress.Tag, nInner.NcsCommitQueueProgress.State)

	// Pick out the completed devices (if any)
	//
	// TODO: Should do something with the failed, transient devices

	for i := range nInner.NcsCommitQueueProgress.CompletedDevices {
		n.Devices = append(n.Devices, nInner.NcsCommitQueueProgress.CompletedDevices[i].Name)
	}

	return msg, nil
}

//**********
//NETCONF stream event handler
//**********
//
// From nso:ietf-netconf-notifications.yang

type NetconfConfigChange struct {
	//XMLName           xml.Name             `xml:"netconf-config-change" json:"-"`
	User      string                     `xml:"changed-by>username" json:"-"`
	Host      string                     `xml:"changed-by>source-host" json:"-"`
	Datastore string                     `xml:"datastore" json:"-"`
	Edits     []*NetconfConfigChangeEdit `xml:"edit" json:"-"`
}

type NetconfConfigChangeEdit struct {
	//XMLName xml.Name `xml:"edit" json:"-"`
	Target    string `xml:"target" json:"target"`
	Operation string `xml:"operation" json:"operation"`
}

type NetconfSessionStart struct {
	//XMLName           xml.Name             `xml:"netconf-session-start" json:"-"`
	User      string `xml:"username" json:"-"`
	SessionId int    `xml:"session-id" json:"-"`
	Host      string `xml:"source-host" json:"-"`
}

var reDevice = regexp.MustCompile("ncs:device\\[ncs:name='([^']+)'\\]")

func (n *Notification) handlerNetconf(sub streamSubscriber) (string, error) {
	nInner := new(NotificationInner)

	// debugMsgf("[%s] (Notification:handlerNetconf) inner:\n%s\n",
	//      stringColorize(sub.stream.Name, COLOR_STREAM), xmlInnerCleanup(n.Inner))

	// Fake a root node so unmarshal is happy. I guess the notification could all be decoded
	// before this point into the outer Notification structure, but does it make sense to
	// decode the rest into the main structure here? The main structure already has a root
	// node...

	// TODO: Do there really need to be 2 Notification structures?

	rootInner := append(append([]byte("<root>"), n.Inner[:]...), []byte("</root>")...)
	err := xml.Unmarshal(rootInner, &nInner)
	if err != nil {
		return "", err
	}

	// Event = netconf-session-start

	if nInner.NetconfSessionStart != nil {
		n.setEventType(EVENT_NETCONF_SESSION_START)
		n.User = nInner.NetconfSessionStart.User
		n.UserHost = nInner.NetconfSessionStart.Host
		msg := fmt.Sprintf("[%s]", stringColorize(n.EventType.String(), COLOR_EVENT))
		msg = msg + fmt.Sprintf(" [%s %s@%s]", stringColorize("user", COLOR_HIGHLIGHT), n.User, n.UserHost)
		return msg, nil
	}

	// Event = netconf-config-change

	if nInner.NetconfConfigChange == nil {
		return fmt.Sprintf("(handler) no known event data found!"), nil
	}

	n.setEventType(EVENT_NETCONF_CONFIG_CHANGE)
	n.User = nInner.NetconfConfigChange.User
	n.UserHost = nInner.NetconfConfigChange.Host
	n.Datastore = nInner.NetconfConfigChange.Datastore
	msg := fmt.Sprintf("[%s]", stringColorize(n.EventType.String(), COLOR_EVENT))
	msg = msg + fmt.Sprintf(" [%s %s@%s]", stringColorize("user", COLOR_HIGHLIGHT), n.User, n.UserHost)

	// Look through the list of edits attached to this event, extracting all the unique
	// device names and their associated edit targets and operations. The Notification
	// will end up with two copies of each edit, with one set grouped by device

	for i := range nInner.NetconfConfigChange.Edits {
		// Force a copy
		// Is this necessary? The life of the nInner should match the life of Notification
		edit := &NetconfConfigChangeEdit{
			Target:    nInner.NetconfConfigChange.Edits[i].Target,
			Operation: nInner.NetconfConfigChange.Edits[i].Operation,
		}
		n.Edits = append(n.Edits, edit)

		if subMatches := reDevice.FindStringSubmatch(edit.Target); subMatches != nil {
			devName := subMatches[1]
			if index := sort.SearchStrings(n.Devices, devName); index == len(n.Devices) {
				n.Devices = append(n.Devices, devName)
				sort.Strings(n.Devices)
			}
			n.DeviceEdits[devName] = append(n.DeviceEdits[devName], edit)
		} else {
			n.DeviceEdits["none"] = append(n.DeviceEdits["none"], edit)
		}

		msg = msg + fmt.Sprintf("\n%s: %s",
			stringColorize(edit.Operation, COLOR_HIGHLIGHT), edit.Target)
	}

	return msg, nil
}

//**********
// Notification-related utility functions
//**********

// xml.Decode appears to leave SSE "data:" markers in the innerxml

func xmlInnerCleanup(source []byte) []byte {
	return bytes.ReplaceAll(bytes.ReplaceAll(source, []byte("\ndata:     "), []byte("")), []byte("\ndata: "), []byte(""))
}

// Enrich the event data with various tidbits. The Devices and Edits fields are slightly
// redundant, but having Devices allows easier access to just the device names

type enrichData struct {
	Source    string                                `json:"source"`
	Stream    string                                `json:"stream"`
	EventName string                                `json:"eventname"`
	User      string                                `json:"user,omitempty"`
	Host      string                                `json:"host,omitempty"`
	Datastore string                                `json:"datastore,omitempty"`
	Devices   []string                              `json:"devices,omitempty"`
	Edits     map[string][]*NetconfConfigChangeEdit `json:"edits,omitempty"`
	Event     string                                `json:"event"`
}

// Custom alternative to json.Marshal() that explicitly turns off escaping of <, > (and ampersand)

func jsonMarshal(t interface{}) ([]byte, error) {
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false) // The key difference
	err := encoder.Encode(t)
	return buffer.Bytes(), err
}

func (n *Notification) enrichData(sub streamSubscriber, source []byte) []byte {

	// Build the body from the various tidbits, including the entire original XML event
	// structure in case something wants more detail

	body := &enrichData{
		Source:    sub.url.Host,
		Stream:    sub.stream.Name,
		EventName: n.EventName,
		User:      n.User,
		Host:      n.UserHost,
		Datastore: n.Datastore,
		Devices:   n.Devices,
		Edits:     n.DeviceEdits,
		Event:     "<nsoevent>" + string(source) + "</nsoevent>",
	}

	result, err := jsonMarshal(body)
	if err != nil {
		panic(fmt.Errorf("(Notification:enrichData) jsonMarshal"))
	}

	debugMsgf("[%s] (Notification:enrichData) result '%s'\n",
		stringColorize(sub.stream.Name, COLOR_STREAM), result)

	return result
}
