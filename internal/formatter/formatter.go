package formatter

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/fatih/color"
	"gopkg.in/yaml.v3"
)

// Format represents the output format
type Format string

const (
	// FormatText represents text format
	FormatText Format = "text"
	// FormatJSON represents JSON format
	FormatJSON Format = "json"
	// FormatYAML represents YAML format
	FormatYAML Format = "yaml"
)

// Formatter formats the output
type Formatter struct {
	Format Format
	Writer io.Writer
	Color  bool
}

// NewFormatter creates a new formatter
func NewFormatter(format string, writer io.Writer, useColor bool) *Formatter {
	f := Format(strings.ToLower(format))
	if f != FormatText && f != FormatJSON && f != FormatYAML {
		f = FormatText
	}

	return &Formatter{
		Format: f,
		Writer: writer,
		Color:  useColor,
	}
}

// Print prints the data in the specified format
func (f *Formatter) Print(data interface{}) error {
	switch f.Format {
	case FormatJSON:
		return f.printJSON(data)
	case FormatYAML:
		return f.printYAML(data)
	default:
		return f.printText(data)
	}
}

// printJSON prints the data in JSON format
func (f *Formatter) printJSON(data interface{}) error {
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(f.Writer, string(bytes))
	return err
}

// printYAML prints the data in YAML format
func (f *Formatter) printYAML(data interface{}) error {
	bytes, err := yaml.Marshal(data)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(f.Writer, string(bytes))
	return err
}

// printText prints the data in text format
func (f *Formatter) printText(data interface{}) error {
	switch v := data.(type) {
	case map[string]interface{}:
		return f.printMap(v, 0)
	case []interface{}:
		return f.printSlice(v, 0)
	default:
		_, err := fmt.Fprintln(f.Writer, v)
		return err
	}
}

// printMap prints a map in text format
func (f *Formatter) printMap(m map[string]interface{}, indent int) error {
	for k, v := range m {
		indentStr := strings.Repeat("  ", indent)
		key := k
		if f.Color {
			key = color.CyanString(k)
		}

		switch val := v.(type) {
		case map[string]interface{}:
			_, err := fmt.Fprintf(f.Writer, "%s%s:\n", indentStr, key)
			if err != nil {
				return err
			}
			err = f.printMap(val, indent+1)
			if err != nil {
				return err
			}
		case []interface{}:
			_, err := fmt.Fprintf(f.Writer, "%s%s:\n", indentStr, key)
			if err != nil {
				return err
			}
			err = f.printSlice(val, indent+1)
			if err != nil {
				return err
			}
		case time.Time:
			_, err := fmt.Fprintf(f.Writer, "%s%s: %s\n", indentStr, key, val.Format(time.RFC3339))
			if err != nil {
				return err
			}
		default:
			_, err := fmt.Fprintf(f.Writer, "%s%s: %v\n", indentStr, key, val)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// printSlice prints a slice in text format
func (f *Formatter) printSlice(s []interface{}, indent int) error {
	for i, v := range s {
		indentStr := strings.Repeat("  ", indent)
		switch val := v.(type) {
		case map[string]interface{}:
			_, err := fmt.Fprintf(f.Writer, "%s%d:\n", indentStr, i)
			if err != nil {
				return err
			}
			err = f.printMap(val, indent+1)
			if err != nil {
				return err
			}
		case []interface{}:
			_, err := fmt.Fprintf(f.Writer, "%s%d:\n", indentStr, i)
			if err != nil {
				return err
			}
			err = f.printSlice(val, indent+1)
			if err != nil {
				return err
			}
		default:
			_, err := fmt.Fprintf(f.Writer, "%s- %v\n", indentStr, val)
			if err != nil {
				return err
			}
		}
	}
	return nil
}