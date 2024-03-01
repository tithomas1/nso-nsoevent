/*
Author:  Tim Thomas
Created: 23-Sep-2020
*/

package main

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

const (
	requestURLState = "/data/tailf-ncs-monitoring:ncs-state"
)

/*
 Structure definitions based on what the top-level list of available streams looks like
 coming back from NSO.

 XML example:

<ncs-state xmlns="http://tail-f.com/yang/ncs-monitoring" xmlns:tfnm="http://tail-f.com/yang/ncs-monitoring">
  <version>5.4.1</version>
  <smp>
    <number-of-threads>2</number-of-threads>
  </smp>
  <epoll>true</epoll>
  <daemon-status>started</daemon-status>
  <loaded-data-models>
    <data-model>
      <name>CARE</name>
      <revision>2020-08-31</revision>
      <namespace>http://example.com/CARE</namespace>
      <prefix>CARE</prefix>
      <exported-to-all/>
    </data-model>
    etc...
  </loaded-data-model>
</ncs-state>
*/

type State struct {
	XMLName          xml.Name         `xml:"ncs-state"`
	Version          string           `xml:"version"`
	LoadedDataModels LoadedDataModels `xml:"loaded-data-models"`
	Internal         Internal         `xml:"internal"`
}

type DataModelList []*DataModel
type MountList []*Mount

type LoadedDataModels struct {
	XMLName       xml.Name      `xml:"loaded-data-models"`
	DataModelList DataModelList `xml:"data-model"`
	MountList     MountList     `xml:"mount"`
}

type DataModel struct {
	XMLName   xml.Name  `xml:"data-model"`
	Name      string    `xml:"name"`
	Revision  string    `xml:"revision"`
	Namespace string    `xml:"namespace"`
	Prefix    string    `xml:"prefix"`
	mountList MountList `xml:"-"`
}

type Mount struct {
	XMLName       xml.Name      `xml:"mount"`
	MountId       string        `xml:"mount-id"`
	DataModelList DataModelList `xml:"data-model"`
}

type DatastoreList []*Datastore
type Callpoints []*Callpoint
type Actionpoints []*Actionpoint
type Callbacks []string

type Internal struct {
	XMLName      xml.Name     `xml:"internal"`
	Callpoints   Callpoints   `xml:"callpoints>callpoint"`
	Actionpoints Actionpoints `xml:"callpoints>actionpoint"`
	CDB          CDB          `xml:"cdb"`
}

type CDB struct {
	XMLName       xml.Name      `xml:"cdb"`
	DatastoreList DatastoreList `xml:"datastore"`
}

type Datastore struct {
	XMLName  xml.Name `xml:"datastore"`
	Name     string   `xml:"name"`
	Filename string   `xml:"filename"`
	DiskSize string   `xml:"disk-size"`
	RAMSize  string   `xml:"ram-size"`
}

type Callpoint struct {
	XMLName xml.Name `xml:"callpoint"`
	Id      string   `xml:"id"`
	Daemon  Daemon   `xml:"daemon"`
	Error   string   `xml:"error"`
}

type Actionpoint struct {
	XMLName xml.Name `xml:"actionpoint"`
	Id      string   `xml:"id"`
	Daemon  Daemon   `xml:"daemon"`
}

type Daemon struct {
	XMLName   xml.Name  `xml:"daemon"`
	Id        string    `xml:"id"`
	Name      string    `xml:"name"`
	Callbacks Callbacks `xml:"callbacks"`
}

// Retrieve current NSO server state

func (s *NsoServer) getState() error {
	data, _, err := s.getResourceData(requestURLState)
	if err != nil {
		return err
	}

	// TODO Seen one case where NSO returned XML that caused an Unmarshal error. Should print offending content
	state := new(State)
	err = xml.Unmarshal(data, &state)
	if err != nil {
		fmt.Printf("(getState) xml.Unmarshal: %v", err)
		return err
	}

	s.Version = state.Version
	s.State = state

	// Expand the list of data models to include any new ones referenced in the mounts
	for _, mount := range s.State.LoadedDataModels.MountList {
		for _, mountModel := range mount.DataModelList {
			if s.State.LoadedDataModels.findByName(mountModel.Name) == nil {
				s.State.LoadedDataModels.DataModelList = append(s.State.LoadedDataModels.DataModelList, mountModel)
			}
		}
	}
	return nil
}

func (s *NsoServer) dataModelCount() int {
	return s.State.LoadedDataModels.count()
}

func (s *NsoServer) datastoreCount() int {
	return s.State.Internal.CDB.DatastoreList.count()
}

func (s *NsoServer) mountCount() int {
	return s.State.LoadedDataModels.MountList.count()
}

func (s *NsoServer) callpointCount() int {
	return s.State.Internal.Callpoints.count()
}

func (s *NsoServer) actionpointCount() int {
	return s.State.Internal.Actionpoints.count()
}

func (s *NsoServer) streamCount() int {
	return s.StreamList.count()
}

func (s *NsoServer) printModelList() {
	s.State.LoadedDataModels.print()
}

func (s *NsoServer) printStreamList() {
	s.StreamList.print()
}

func (s *NsoServer) printDatastoreList() {
	s.State.Internal.CDB.DatastoreList.print()
}

func (s *NsoServer) printMountList() {
	s.State.LoadedDataModels.MountList.print()
}

func (s *NsoServer) printCallpoints() {
	s.State.Internal.Callpoints.print()
}

func (s *NsoServer) printActionpoints() {
	s.State.Internal.Actionpoints.print()
}

func (l LoadedDataModels) count() int {
	return len(l.DataModelList)
}

func (l LoadedDataModels) findByName(n string) *DataModel {
	for _, d := range l.DataModelList {
		if n == d.Name {
			return d
		}
	}
	return nil
}

func (l LoadedDataModels) print() {
	count := len(l.DataModelList)
	if count == 0 {
		return
	}

	if Config.showMounts {
		// Determine which data models the mounts are referencing
		for _, mount := range l.MountList {
			for _, mountModel := range mount.DataModelList {
				if foundModel := l.findByName(mountModel.Name); foundModel != nil {
					debugMsgf("(LoadedDataModels:print) adding mount %s to %s\n", mount.MountId, foundModel.Name)
					foundModel.mountList = append(foundModel.mountList, mount)
				}
			}
		}
	}

	// Determine max column widths
	width := []int{0, 0, 10, 0} // Revision is yyyy-mm-dd
	for _, item := range l.DataModelList {
		width = findMaxStringWidths(width, item.Name, item.Prefix)
	}
	for i := 0; i < len(width)-1; i++ {
		width[i] = width[i] + outputColumnPadding
	}

	index := 0
	tablePrint(
		//&[]int{w1, w2, w3, 0},
		&width,
		&[]string{"Name", "Prefix", "Revision", "Namespace"},
		func() (*[]string, string) {
			if index >= count {
				return nil, ""
			}
			item := l.DataModelList[index]
			index++
			modelColor := COLOR_CYAN
			if strings.Contains(item.Namespace, "ned") &&
				!(strings.Contains(item.Namespace, "ned-secrets") ||
					strings.Contains(item.Namespace, "ncs-ned") ||
					strings.Contains(item.Namespace, "netconf-ned-builder")) {
				modelColor = COLOR_YELLOW
			}
			if strings.Contains(item.Namespace, "example") {
				modelColor = COLOR_RED
			}
			columns := []string{
				stringColorize(item.Name, modelColor),
				stringColorize(item.Prefix, modelColor),
				stringColorize(item.Revision, modelColor),
				stringColorize(item.Namespace, modelColor),
			}
			extra := ""
			if Config.showMounts {
				for _, mount := range item.mountList {
					extra = extra + fmt.Sprintf(stringColorize(fmt.Sprintf("  --> mount: %s\n", mount.MountId), COLOR_HI_WHITE))
				}
			}
			return &columns, extra
		})

	fmt.Printf("\n%s loaded data model%s\n", stringColorize(strconv.Itoa(count), COLOR_HI_YELLOW), pluralSuffix(count))

}

func (l DatastoreList) count() int {
	return len(l)
}

func (l DatastoreList) print() {
	count := len(l)
	if count == 0 {
		return
	}

	// Determine max column widths
	width := []int{0, 12, 12, 0} // 12 should be pretty big for RAM/disk size
	for _, item := range l {
		width = findMaxStringWidths(width, item.Name)
	}
	for i := 0; i < len(width)-1; i++ {
		width[i] = width[i] + outputColumnPadding
	}

	index := 0
	tablePrint(
		&width,
		&[]string{"Name", "RAM size", "Disk size", "Filename"},
		func() (*[]string, string) {
			if index >= count {
				return nil, ""
			}
			item := l[index]
			index++
			columns := []string{
				stringColorize(item.Name, COLOR_YELLOW),
				stringColorize(item.RAMSize, COLOR_CYAN),
				stringColorize(item.DiskSize, COLOR_CYAN),
				stringColorize(item.Filename, COLOR_CYAN),
			}
			return &columns, ""
		})

	fmt.Printf("\n%s datastore%s\n", stringColorize(strconv.Itoa(count), COLOR_HI_YELLOW), pluralSuffix(count))
}

func (l MountList) count() int {
	return len(l)
}

func (l MountList) print() {
	count := len(l)
	if count == 0 {
		return
	}

	// Determine max column widths
	width := []int{0, 0, 0}
	for _, item := range l {
		width = findMaxStringWidths(width, item.MountId, "Unknown")
	}
	for i := 0; i < len(width)-1; i++ {
		width[i] = width[i] + outputColumnPadding
	}

	index := 0
	tablePrint(
		&width,
		&[]string{"Id", "Data model"},
		func() (*[]string, string) {
			if index >= count {
				return nil, ""
			}
			item := l[index]
			index++
			columns := []string{
				stringColorize(item.MountId, COLOR_CYAN),
				stringColorize("Unknown", COLOR_CYAN),
			}
			return &columns, ""
		})

	fmt.Printf("\n%s mount%s\n", stringColorize(strconv.Itoa(count), COLOR_HI_YELLOW), pluralSuffix(count))
}

func (l Callpoints) count() int {
	return len(l)
}

func (l Callpoints) print() {
	count := len(l)
	if count == 0 {
		return
	}

	// Determine max column widths
	width := []int{0, 0, 0}
	for _, item := range l {
		width = findMaxStringWidths(width, item.Id, item.Daemon.Name)
	}
	for i := 0; i < len(width)-1; i++ {
		width[i] = width[i] + outputColumnPadding
	}

	index := 0
	errCount := 0
	tablePrint(
		&width,
		&[]string{"Id", "Daemon", "Callbacks"},
		func() (*[]string, string) {
			if index >= count {
				return nil, ""
			}
			item := l[index]
			index++
			itemColor := COLOR_CYAN
			if item.Error != "" {
				itemColor = COLOR_ERROR
				errCount++
			}
			callbacks := ""
			for i, callback := range item.Daemon.Callbacks {
				if i > 0 {
					callbacks = callbacks + ","
				}
				callbacks = callbacks + callback
			}
			columns := []string{
				stringColorize(item.Id, itemColor),
				stringColorize(item.Daemon.Name, itemColor),
				stringColorize(callbacks, itemColor),
			}
			return &columns, ""
		})

	// If any callbacks had error tags
	if errCount > 0 {
		fmt.Println(stringColorize("\nErrors:", COLOR_HEADINGS))
		for _, item := range l {
			if item.Error != "" {
				fmt.Println(stringColorize(fmt.Sprintf("%-*s%s", width[0], item.Id, item.Error), COLOR_ERROR))
			}
		}
	}

	fmt.Printf("\n%s callpoint%s\n", stringColorize(strconv.Itoa(count), COLOR_HI_YELLOW), pluralSuffix(count))
}

func (l Actionpoints) count() int {
	return len(l)
}

func (l Actionpoints) print() {
	count := len(l)
	if count == 0 {
		return
	}

	// Determine max column widths

	width := []int{0, 0, 0}
	for _, item := range l {
		width = findMaxStringWidths(width, item.Id, item.Daemon.Name)
	}
	for i := 0; i < len(width)-1; i++ {
		width[i] = width[i] + outputColumnPadding
	}

	index := 0
	tablePrint(
		&width,
		&[]string{"Id", "Daemon", "Callbacks"},
		func() (*[]string, string) {
			if index >= count {
				return nil, ""
			}
			item := l[index]
			index++
			itemColor := COLOR_CYAN
			callbacks := ""
			for i, callback := range item.Daemon.Callbacks {
				if i > 0 {
					callbacks = callbacks + ","
				}
				callbacks = callbacks + callback
			}
			columns := []string{
				stringColorize(item.Id, itemColor),
				stringColorize(item.Daemon.Name, itemColor),
				stringColorize(callbacks, itemColor),
			}
			return &columns, ""
		})

	fmt.Printf("\n%s actionpoint%s\n", stringColorize(strconv.Itoa(count), COLOR_HI_YELLOW), pluralSuffix(count))
}
