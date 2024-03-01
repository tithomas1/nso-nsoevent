/*
Author:  Tim Thomas
Created: 21-Sep-2020
*/

package main

import (
	"fmt"
)

var debugOutput = false

func enableDebug() {
	debugOutput = true
	fmt.Println(stringColorize("Debug output enabled", COLOR_RED))
}

/*
 * Logging debug output to default for now
 */
func debugMsgf(msg string, v ...interface{}) {
	if debugOutput {
		fmt.Printf(msg, v...)
	}
}
