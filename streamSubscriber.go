/*
Author:  Tim Thomas
Created: 24-Sep-2020
*/

package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"
)

type ioStream struct {
	reader io.ReadCloser
}

type streamSubscriber struct {
	done       func() <-chan struct{}
	stream     *Stream
	url        *url.URL
	ioStream   ioStream
	handler    func(*Notification, streamSubscriber) (string, error)
	eventCount int
}

type subscriberList []*streamSubscriber

var streamSubscriberList subscriberList

func (s *NsoServer) startSubscribers() error {

	// Set up list of subscribers, only to the XML streams for now. If no specific stream(s)
	// were requested, then assume all

	if len(Config.streamNames) == 0 {
		for _, availStream := range s.StreamList.Stream {
			Config.streamNames = append(Config.streamNames, availStream.Name)
		}
	}

	found := map[string]bool{}

	for _, requestStream := range Config.streamNames {
		found[requestStream] = false
		for _, availStream := range s.StreamList.Stream {
			if fuzzyNameMatch(requestStream, availStream.Name) {
				for _, a := range availStream.Access {
					if a.EncodingType == ENCODING_XML {
						found[requestStream] = true
						streamSubscriberList = append(streamSubscriberList, &streamSubscriber{stream: availStream, url: a.LocationURL, handler: (*Notification).handlerDefault})
					}
				}
			}
		}
	}

	// Were any requested streams not found?

	if len(Config.streamNames) > len(streamSubscriberList) {
		for n, f := range found {
			if !f {
				fmt.Printf("stream '%s' not found\n", stringColorize(n, COLOR_ERROR))
			}
		}
		return fmt.Errorf("stream(s) not found")
	}

	// Register handlers for the known stream types

	streamSubscriberList.registerHandler("ncs-events", (*Notification).handlerNcsEvents)
	streamSubscriberList.registerHandler("NETCONF", (*Notification).handlerNetconf)

	// Set up a master context that can stop all subscribers

	cancelCtx, cancelSubscribers := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	// Start the individual subscribers

	for i := range streamSubscriberList {
		streamSubscriberList[i].done = cancelCtx.Done
		wg.Add(1)
		go func(sub streamSubscriber) {
			defer wg.Done()
			events, err := s.startSubscriber(sub)
			if err != nil {
				fmt.Printf("[%s] (startSubscriber) %s after %s event%s - %s\n",
					stringColorize(sub.stream.Name, COLOR_STREAM),
					stringColorize("exiting", COLOR_ERROR),
					stringColorize(strconv.Itoa(events), COLOR_HIGHLIGHT), pluralSuffix(events),
					stringColorize(fmt.Sprintf("ERROR: %v", err), COLOR_ERROR))
				// TODO should an error from an individual subscriber cancel them all?
			} else {
				fmt.Printf("[%s] (startSubscriber) %s after %s event%s\n",
					stringColorize(sub.stream.Name, COLOR_STREAM), stringColorize("exiting", COLOR_ERROR),
					stringColorize(strconv.Itoa(events), COLOR_HIGHLIGHT), pluralSuffix(events))
			}
		}(*streamSubscriberList[i])
	}

	// Wait for all the subscribers to exit, possibly from a control-C

	go func() {
		c := make(chan os.Signal)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		<-c
		cancelSubscribers()
		wg.Wait()
	}()

	wg.Wait()
	return nil
}

func (sl subscriberList) registerHandler(streamName string, h func(*Notification, streamSubscriber) (string, error)) {
	for _, s := range sl {
		if s.stream.Name == streamName {
			s.handler = h
		}
	}
}

// Primary stream subscriber and high-level event code

func (s *NsoServer) startSubscriber(sub streamSubscriber) (int, error) {
	var err error

	fmt.Printf("[%s] (NSOServer:startSubscriber) %s\n", stringColorize(sub.stream.Name, COLOR_STREAM), stringColorize(sub.url.String(), COLOR_URL))

	sub.ioStream.reader, err = s.openStream(sub.url)
	if err != nil {
		return 0, err
	}
	defer sub.ioStream.reader.Close()

	d := xml.NewDecoder(sub.ioStream.reader)

	// Wrap the event reader in a goroutine to allow checking for done signal too

	notificationChan := make(chan Notification, 1) // TODO Should the notification queue be > 1?

	// Concurrent func to receive incoming events, wrap them in a Notification, and publish
	// them to an outgoing Notification channel
	go func(out chan<- Notification) {
		defer close(out)
		for {
			n := newNotification()
			if err := d.Decode(&n); err != nil { // Can happen if/when sub.ioStream.reader closes
				break
			}
			out <- *n
			n = nil // Hint to garbage collection
		}
	}(notificationChan)

	// Wait for event notifications from the channel, only decoding the outer tag directly from the
	// stream. The subordinate contents will be saved in Notification.Inner
	for {
		select {
		case <-sub.done():
			return sub.eventCount, nil

		case n, ok := <-notificationChan:
			if !ok {
				return sub.eventCount, fmt.Errorf("notification channel closed/unavailable")
			}

			// The registered stream handler will interpret the message

			if sub.handler != nil {
				sub.eventCount++
				go func(n *Notification) {
					logMsg := fmt.Sprintf("[%s] %s", stringColorize(sub.stream.Name, COLOR_STREAM), stringColorize(n.EventTime.Format(time.RFC3339), COLOR_HIGHLIGHT))
					msg, err := sub.handler(n, sub)
					if err == nil {
						fmt.Println(logMsg + " " + msg)

						// Fire the associated webhooks

						innerClean := n.enrichData(sub, xmlInnerCleanup(n.Inner))
						for _, hook := range sub.stream.Webhooks {
							if hook.shouldFire(n, innerClean) {
								go func(w webhook) {
									w.fire(sub, innerClean)
								}(*hook)
							}
						}
					} else {
						// TODO: Should a handler error cause the subscriber to exit?
						fmt.Println(logMsg + stringColorize(" handler ERROR: ", COLOR_ERROR) + err.Error())
						//return
					}
				}(&n)
			} else {
				fmt.Printf("[%s] no handler registered!\n", stringColorize(sub.stream.Name, COLOR_STREAM))
			}
		}
	}
}
