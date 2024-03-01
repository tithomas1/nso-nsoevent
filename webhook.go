/*
Author:  Tim Thomas
Created: 23-Oct-2020
*/

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
)

type Filter struct {
	Event string
	Node  []*map[string]string
}

type webhook struct {
	Stream   string
	Disable  bool
	Url      string
	User     string
	ApiToken string
	Token    string
	//Filter     map[string]string
	Filter     *Filter
	StreamList []*Stream
	targetURL  *url.URL
}

type webhooks []*webhook

func (webhooks webhooks) validate(streamList *StreamList) {
	if hookCount := len(webhooks); hookCount > 0 {

		// Check the webhook URLs
		for _, hook := range webhooks {
			targetUrl, err := url.Parse(hook.Url)
			if err != nil {
				fmt.Printf("%s: config webhook URL '%s': %v\n",
					stringColorize("ERROR", COLOR_ERROR), stringColorize(hook.Url, COLOR_ERROR), err.Error())
				hook.StreamList = nil
				hook.Disable = true
			} else {
				hook.targetURL = targetUrl
			}

			if hook.Filter != nil {
				for _, n := range hook.Filter.Node {
					value, valueOk := (*n)["value"]
					if valueOk {
						_, err := regexp.Compile(value)
						if err != nil {
							fmt.Printf("%s: config webhook for '%s': invalid filter '%s': regexp('%s')\n",
								stringColorize("ERROR", COLOR_ERROR),
								stringColorize(hook.Stream, COLOR_ERROR),
								stringColorize("value", COLOR_HIGHLIGHT),
								stringColorize(value, COLOR_ERROR))
							hook.StreamList = nil
							hook.Disable = true
						}
					}
				}
			}
		}

		// Find the stream reference for each webhook
		for _, hook := range webhooks {
			hook.StreamList = streamList.findStreamsByName(hook.Stream)
		}

		// Generate a warning if a webhook references no known stream(s). Otherwise link
		// the stream(s) back to the webhook(s)
		for _, hook := range webhooks {
			if hook.StreamList == nil {
				fmt.Printf("%s: config webhook reference to stream(s) '%s' not found on server\n",
					stringColorize("WARNING", COLOR_WARNING), stringColorize(hook.Stream, COLOR_WARNING))
			} else {
				for _, stream := range hook.StreamList {
					stream.addWebhook(hook)
				}
			}
		}
	}
}

func (webhooks webhooks) print() {
	if hookCount := len(webhooks); hookCount > 0 {
		fmt.Printf("%s webhook%s defined\n", stringColorize(strconv.Itoa(hookCount), COLOR_HIGHLIGHT), pluralSuffix(hookCount))
		for _, hook := range webhooks {
			disableFlag := ""
			if hook.Disable {
				disableFlag = stringColorize("(DISABLED)", COLOR_HI_RED)
			}
			streamCount := len(hook.StreamList)
			debugMsgf(" -> %s[%s:", disableFlag, stringColorize(hook.Stream, COLOR_WEBHOOK))
			for i, stream := range hook.StreamList {
				debugMsgf("%s", stringColorize(stream.Name, COLOR_STREAM))
				if (i + 1) < streamCount {
					debugMsgf(",")
				}
			}
			debugMsgf("]: target %s, token %s\n", stringColorize(hook.Url, COLOR_URL), stringColorize(hook.Token, COLOR_HIGHLIGHT))
			if Config.debug {
				hook.Filter.print()
			}
		}
	}
}

func (f *Filter) print() {
	if f != nil {
		if f.Event != "" {
			fmt.Printf("    filter: event = %s\n", f.Event)
		}
		if count := len(f.Node); count > 0 {
			fmt.Printf("    filter: %s node%s\n", stringColorize(strconv.Itoa(count), COLOR_HIGHLIGHT), pluralSuffix(count))
			for _, n := range f.Node {
				fmt.Printf("    filter: %v\n", *n)
			}
		}
	}
}

func (webhook *webhook) fire(sub streamSubscriber, body []byte) {
	fmt.Printf("[%s] (webhook:fire) POST to %s with token '%s'\n",
		stringColorize(sub.stream.Name, COLOR_STREAM),
		stringColorize(webhook.Url, COLOR_URL), webhook.Token)

	debugMsgf("[%s] (webhook:fire) POST body '%s'\n", stringColorize(sub.stream.Name, COLOR_STREAM), body)

	// Construct the POST request
	req := &http.Request{
		Method:     "POST",
		URL:        webhook.targetURL,
		Proto:      "",
		ProtoMajor: 0,
		ProtoMinor: 0,
		Header: map[string][]string{
			"Content-Type": {"application/json"},
			"token":        {webhook.Token},
		},
		Body:  ioutil.NopCloser(bytes.NewBuffer(body)),
		Close: true,
	}

	// Issue the request with a timeout
	ctx, cancel := context.WithTimeout(context.Background(), Config.connectTimeout)
	defer cancel()

	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		if e, ok := err.(net.Error); ok && e.Timeout() {
			fmt.Printf("[%s] (webhook:fire) URL '%s' timeout\n",
				stringColorize(sub.stream.Name, COLOR_STREAM),
				stringColorize(webhook.Url, COLOR_URL))
		} else {
			fmt.Printf("[%s] (webhook:fire) %s: %s\n",
				stringColorize(sub.stream.Name, COLOR_STREAM),
				stringColorize("ERROR", COLOR_ERROR),
				stringColorize(err.Error(), COLOR_ERROR))
		}
		return
	}
	responseData, _ := ioutil.ReadAll(resp.Body)
	_ = resp.Body.Close()

	// Process return

	if resp.StatusCode == http.StatusNotFound {
		fmt.Printf("[%s] (webhook:fire) %s from %s: %s\n",
			stringColorize(sub.stream.Name, COLOR_STREAM),
			stringColorize("NOT FOUND (404)", COLOR_ERROR),
			stringColorize(webhook.Url, COLOR_URL), responseData)
	} else {
		debugMsgf("[%s] (webhook:fire) HTTP %s response from %s: %s\n",
			stringColorize(sub.stream.Name, COLOR_STREAM),
			stringColorize(strconv.Itoa(resp.StatusCode), COLOR_HI_BLUE),
			stringColorize(webhook.Url, COLOR_URL), responseData)

		var responseMap map[string]json.RawMessage
		err = json.Unmarshal(responseData, &responseMap)
		if err != nil {
			fmt.Printf("[%s] (webhook:fire) json.Unmarshall responseData: %v\n",
				stringColorize(sub.stream.Name, COLOR_STREAM), err)
		}

		var jobsMap map[string]json.RawMessage
		err = json.Unmarshal(responseMap["jobs"], &jobsMap)
		if err != nil {
			fmt.Printf("[%s] (webhook:fire) json.Unmarshall responseMap: %v\n",
				stringColorize(sub.stream.Name, COLOR_STREAM), err)
		}

		// Could be multiple pipeline job results
		for k, v := range jobsMap {
			var pipelineMap map[string]json.RawMessage
			err = json.Unmarshal(v, &pipelineMap)
			if err != nil {
				fmt.Printf("[%s] (webhook:fire) %s: json.Unmarshall jobsMap[%s]: %v\n",
					stringColorize(sub.stream.Name, COLOR_STREAM),
					stringColorize("ERROR", COLOR_ERROR), stringColorize(k, COLOR_HIGHLIGHT), err)
			} else {
				fmt.Printf("[%s] (webhook:fire) job '%s' triggered: %s\n",
					stringColorize(sub.stream.Name, COLOR_STREAM),
					stringColorize(k, COLOR_HIGHLIGHT),
					stringColorize(string(pipelineMap["triggered"]), COLOR_HIGHLIGHT))
			}
		}
	}
}

func (webhook webhook) shouldFire(n *Notification, data []byte) bool {
	return !webhook.Disable && webhook.filter(n, data)
}

func (webhook *webhook) filter(n *Notification, data []byte) bool {
	if webhook.Filter == nil {
		return true
	}

	if webhook.Filter.Event != "" && webhook.Filter.Event != n.EventName {
		return false
	}

	for _, n := range webhook.Filter.Node {
		name, nameOk := (*n)["name"]
		value, valueOk := (*n)["value"]
		if nameOk && valueOk {
			reNode := regexp.MustCompile(fmt.Sprintf("<%s[^>]*>%s</%s>", name, value, name))
			if subMatches := reNode.FindStringSubmatch(string(data)); subMatches == nil {
				debugMsgf("[%s] (webhook:filter) did %s find node '%s' with value '%s'\n",
					stringColorize(webhook.Stream, COLOR_STREAM),
					stringColorize("NOT", COLOR_HI_RED),
					stringColorize(name, COLOR_HIGHLIGHT), stringColorize(value, COLOR_HIGHLIGHT))
				return false
			}
		}
		if nameOk && !valueOk { // Node must be present, but value irrelevant
			reNode := regexp.MustCompile(fmt.Sprintf("<%s[\\s>]+", name))
			if subMatches := reNode.FindStringSubmatch(string(data)); subMatches == nil {
				debugMsgf("[%s] (webhook:filter) did not find node '%s'\n",
					stringColorize(webhook.Stream, COLOR_STREAM),
					stringColorize("NOT", COLOR_HI_RED), stringColorize(name, COLOR_HIGHLIGHT))
				return false
			}
		}
	}
	return true
}
