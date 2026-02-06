package output

import (
	"fmt"
	"os"

	"github.com/fatih/color"
)

// Error prints an error message
func Error(err error) {
	fmt.Fprintln(os.Stderr, color.RedString("Error: %s", err.Error()))
}

// Success prints a success message
func Success(message string) {
	fmt.Println(color.GreenString("Success: %s", message))
}

// Warning prints a warning message
func Warning(message string) {
	fmt.Println(color.YellowString("Warning: %s", message))
}

// Info prints an info message
func Info(message string) {
	fmt.Println(color.BlueString("Info: %s", message))
}
