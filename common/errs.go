package common

import (
	"fmt"
	"log"

	"rogchap.com/v8go"
)

// CheckErr is a basic common means to handle errors, can add more logic later.
func CheckErr(err error) {
	if err != nil {
		if isV8Err(err) {
			return
		}
		log.Println(err)
	}
}

// just gives more detal of v8 errors
func isV8Err(err error) bool {
	if e, ok := err.(*v8go.JSError); ok {
		fmt.Println(e.Message)    // the message of the exception thrown
		fmt.Println(e.Location)   // the filename, line number and the column where the eor occured
		fmt.Println(e.StackTrace) // the full stack trace of the error, if available

		fmt.Printf("javascript error: %v\n", err) // will format the standard error message
		fmt.Printf("javascript stack trace: %+v\n", err)
		return true
	}
	return false

}
