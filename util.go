/*
Author:  Tim Thomas
Created: 23-Sep-2020
*/

package main

import (
	"fmt"
	"strings"
)

const ESCAPE = "\033"

const (
	COLOR_RESET = iota
)
const (
	COLOR_BLACK = iota + 30
	COLOR_RED
	COLOR_GREEN
	COLOR_YELLOW
	COLOR_BLUE
	COLOR_PURPLE
	COLOR_CYAN
	COLOR_WHITE
)
const (
	COLOR_HI_BLACK = iota + 90
	COLOR_HI_RED
	COLOR_HI_GREEN
	COLOR_HI_YELLOW
	COLOR_HI_BLUE
	COLOR_HI_PURPLE
	COLOR_HI_CYAN
	COLOR_HI_WHITE
)

const (
	COLOR_ERROR     = COLOR_HI_RED
	COLOR_HIGHLIGHT = COLOR_YELLOW
	COLOR_WARNING   = COLOR_RED
	COLOR_HEADINGS  = COLOR_HI_WHITE
	COLOR_WEBHOOK   = COLOR_HI_BLUE
	COLOR_STREAM    = COLOR_PURPLE
	COLOR_URL       = COLOR_YELLOW
	COLOR_EVENT     = COLOR_HI_BLUE
)

func pluralSuffix(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// NSO's stream locations may need tweaking. For example, 'localhost' should be replaced
// by the actual IP, as well as fixing up port numbers if the target NSO happens to be
// running inside a container
// TODO: What do these locations look like if NSO is running as a cluster?

func fixupHostString(s string) string {
	newS := restconfApiRE.ReplaceAllString(s, Config.nsoTarget.apiUrl)
	if strings.Index(newS, "localhost") == -1 {
		return newS
	}
	return strings.Replace(newS, "localhost", Config.nsoTarget.ipAddress, 1)
}

// Make the comparison a little less exact
func fuzzyNameMatch(s string, target string) bool {
	if s == target || strings.Contains(target, s) {
		return true
	}
	return false
}

// Simple output colors
func stringColorize(s string, c int) string {
	if Config.noColor {
		return s
	}
	return fmt.Sprintf("%s[%dm%s%s[%dm", ESCAPE, c, s, ESCAPE, COLOR_RESET)
}

// Left-justify a string in a given number of spaces, compensating for possible color
func stringColorizeRightPad(s string, l int) string {
	if l == 0 {
		return s
	}
	if Config.noColor {
		return s + strings.Repeat(" ", l-len(s))
	}
	return s + strings.Repeat(" ", l-len(s)+9)
}

func stringRightPad(s string, l int) string {
	if l == 0 {
		return s
	}
	return s + strings.Repeat(" ", l-len(s))
}

func stringMaxLen(l *[]string) int {
	maxLen := 0
	for _, s := range *l {
		if n := len(s); n > maxLen {
			maxLen = n
		}
	}
	return maxLen
}

// Some NSO description strings have embedded newlines and repeated spaces
func stringCleanup(s string) string {
	return strings.ReplaceAll(strings.Replace(s, "\n", " ", -1), "  ", "")
}

func max(n1 int, n2 int) int {
	if n1 > n2 {
		return n1
	}
	return n2
}

func findMaxStringWidths(width []int, s ...string) []int {
	for i, item := range s {
		if l := len(item); l > width[i] {
			width[i] = l
		}
	}
	return width
}

func tablePrint(width *[]int, heading *[]string, next func() (columns *[]string, extra string)) {
	// Column headings
	line := ""
	for i, w := range *width {
		line = line + fmt.Sprintf("%-*s", w, (*heading)[i])
	}
	line = line + "\n"
	for i, w := range *width {
		line = line + fmt.Sprintf("%-*s", w, strings.Repeat("-", len((*heading)[i])))
	}
	fmt.Println(stringColorize(line, COLOR_HEADINGS))

	// Lines/rows
	for {
		columns, extra := next()
		if columns == nil {
			return
		}
		line = ""
		for i, c := range *columns {
			//line = line + fmt.Sprintf("%-*s", (*width)[i], c)
			line = line + fmt.Sprintf("%s", stringColorizeRightPad(c, (*width)[i]))
		}
		fmt.Println(line)
		if extra != "" {
			fmt.Print(extra) // The extra info may have one or more embedded newlines
		}
	}
}
