package main

import (
	"fmt"
	"os"
)

func logError(args ...string) {
	fmt.Printf("[Error]  ")
	fmt.Println(args)
}

func logErrorFatal(args ...string) {
	logError(args...)
	fmt.Println("Exiting...")
	os.Exit(1)
}

func warn(warning string) {
	fmt.Println("[Warning] ", warning)
}

func goodLuck(message string) {
	fmt.Println()
	fmt.Println(message)
	fmt.Println()
}
