/*
Author:  Tim Thomas
Created: 21-Sep-2020
*/

package main

const (
	programName    = "nsoevent"
	programVersion = "0.1.0(alpha)"
)

func main() {
	_ = newBaseCmd().Execute()
}
