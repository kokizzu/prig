package main

/*
Based on a similar idea for Nim:
https://github.com/c-blake/cligen/blob/master/examples/rp.nim

TODO:
- Parse and prettify compile errors
- Add sort helpers to sort slices and map keys/values?
- Add note about which packages are auto-imported? import math, strings, etc
  + or consider using goimports to do this automatically? test performance hit
- Have mode to print (gofmt'd?) source
- Have mode to keep executable

*/

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

const version = "v0.1.0"

func main() {
	// Parse command line arguments
	if len(os.Args) <= 1 {
		errorf(usage)
	}

	var begin []string
	var end []string
	var perRecord []string
	fieldSep := " "

	for i := 1; i < len(os.Args); {
		arg := os.Args[i]
		i++

		switch arg {
		case "-b":
			if i >= len(os.Args) {
				errorf("-b requires an argument")
			}
			begin = append(begin, os.Args[i])
			i++
		case "-e":
			if i >= len(os.Args) {
				errorf("-e requires an argument")
			}
			end = append(end, os.Args[i])
			i++
		case "-F":
			if i >= len(os.Args) {
				errorf("-F requires an argument")
			}
			fieldSep = os.Args[i]
			i++
		case "-i":
			imports[os.Args[i]] = struct{}{}
			if i >= len(os.Args) {
				errorf("-e requires an argument")
			}
			i++
		case "-h", "--help":
			fmt.Println(usage)
			return
		case "-V", "--version":
			fmt.Println(version)
			return
		default:
			switch {
			case strings.HasPrefix(arg, "-F"):
				fieldSep = arg[2:]
			default:
				perRecord = append(perRecord, arg)
			}
		}
	}

	// Create a temporary work directory and .go file
	tempDir, err := os.MkdirTemp("", "prig_")
	if err != nil {
		errorf("error creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	goFilename := filepath.Join(tempDir, "main.go")
	source, err := os.Create(goFilename)
	if err != nil {
		errorf("error creating temp file: %v", err)
	}

	// Write source code to .go file
	err = sourceTemplate.Execute(source, &templateParams{
		FieldSep:  fieldSep,
		Imports:   imports,
		Begin:     begin,
		PerRecord: perRecord,
		End:       end,
	})
	if err != nil {
		errorf("error executing template: %v", err)
	}
	err = source.Close()
	if err != nil {
		errorf("error closing temp file: %v", err)
	}

	// Ensure that Go is installed
	_, err = exec.LookPath("go")
	if err != nil {
		errorf("You must install Go to use 'prig', see https://go.dev/doc/install")
	}

	// Build the program with "go build"
	exeFilename := filepath.Join(tempDir, "main")
	cmd := exec.Command("go", "build", "-o", exeFilename, goFilename)
	output, err := cmd.CombinedOutput()
	switch err := err.(type) {
	case nil:
	case *exec.ExitError:
		// TODO: parse and prettify compile errors?
		b, _ := os.ReadFile(goFilename)
		fmt.Fprint(os.Stderr, string(b), "\n", string(output))
		os.Exit(1)
	default:
		errorf("error building program: %v", err)
	}

	// Then run the executable we just built
	cmd = exec.Command(exeFilename)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		exitCode := cmd.ProcessState.ExitCode()
		if exitCode == -1 {
			errorf("error running program: %v", err)
		}
		os.Exit(exitCode)
	}
}

func errorf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

const usage = `Prig ` + version + ` - Copyright (c) 2022 Ben Hoyt

Prig is for Processing Records In Go. It's like AWK, but snobbish (Go! static
typing!). It runs 'begin code' first, then runs 'per-record code' for every
record (line) in the input, then runs 'end code'. Prig uses "go build", so it
requires the Go compiler: https://go.dev/doc/install

Usage: prig [options] [-b 'begin code'] 'per-record code' [-e 'end code']

Options:
  -b 'begin code'    Go code to run before processing input (multiple allowed)
  -e 'end code'      Go code to run after processing input (multiple allowed)
  -F char | re       field separator (single character or multi-char regex)
  -h, --help         show help message and exit
  -i import          add Go import
  -V, --version      show version number and exit

Built-in functions:
  F(i int) string         // return field i (starts at 1; 0 is current record)
  Float(s string) float64 // convert string to float64 (or return 0.0)
  Int(s string) int       // convert string to int (or return 0)
  NF() int                // return number of fields in current record
  NR() int                // return number of current record

  Replace(re, s, repl string) string  // replace all re matches in s with repl
  Match(re, s string) bool            // report whether s contains match of re
  Submatches(re, s string) []string   // return slice of submatches of re in s
  Substr(s string, n[, m] int) string // s[n:m] but safe and allow negative n/m

  Print(args ...interface{})                 // fmt.Print, but buffered
  Printf(format string, args ...interface{}) // fmt.Printf, but buffered
  Println(args ...interface{})               // fmt.Println, but buffered

Examples: (TODO: test these)
  # Say hi to the world
  prig -b 'Println("Hello, world!")'

  # Print 5th field in milliseconds if record contains "GET" or "HEAD"
  prig 'if Match(` + "`" + `GET|HEAD` + "`" + `, F(0)) { Printf("%.0fms\n", Float(F(5))*1000) }'

  # Print frequencies of unique words in input
  prig -b 'freqs := map[string]int{}' \
          'for i := 1; i <= NF(); i++ { freqs[Lower(F(i))]++ }' \
       -e 'for k, v := range freqs { Println(k, v) }'
`

var imports = map[string]struct{}{
	"bufio":   {},
	"fmt":     {},
	"os":      {},
	"regexp":  {},
	"strconv": {},
	"strings": {},
}

type templateParams struct {
	FieldSep  string
	Imports   map[string]struct{}
	Begin     []string
	PerRecord []string
	End       []string
}

var sourceTemplate = template.Must(template.New("source").Parse(`
package main

import (
{{range $imp, $_ := .Imports}}
{{printf "%q" $imp}}
{{end}}
)

var (
	_output *bufio.Writer
	_record string
	_nr     int
    _fields []string
)

func main() {
	_output = bufio.NewWriter(os.Stdout)
	defer _output.Flush()

{{range .Begin}}
{{.}}
{{end}}

{{if or .PerRecord .End}}
	_scanner := bufio.NewScanner(os.Stdin)
	for _scanner.Scan() {
		_record = _scanner.Text()
        _nr++
        _fields = nil

{{range .PerRecord}}
{{.}}
{{end}}
	}
	if _scanner.Err() != nil {
		_errorf("error reading stdin: %v", _scanner.Err())
	}
{{end}}

{{range .End}}
{{.}}
{{end}}
}

func Print(args ...interface{}) {
	_, err := fmt.Fprint(_output, args...)
	if err != nil {
		_errorf("error writing output: %v", err)
	}
}

func Printf(format string, args ...interface{}) {
	_, err := fmt.Fprintf(_output, format, args...)
	if err != nil {
		_errorf("error writing output: %v", err)
	}
}

func Println(args ...interface{}) {
	_, err := fmt.Fprintln(_output, args...)
	if err != nil {
		_errorf("error writing output: %v", err)
	}
}

func NR() int {
	return _nr
}

func F(i int) string {
	if i == 0 {
		return _record
	}
	_ensureFields()
    if i < 1 || i > len(_fields) {
        return ""
    }
    return _fields[i-1]
}

func Int(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func Float(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

var _fieldSepRegex *regexp.Regexp

func _ensureFields() {
	if _fields != nil {
		return
	}
{{if eq .FieldSep " "}}
	_fields = strings.Fields(_record)
{{else}}
	if _record == "" {
		_fields = []string{}
		return
	}
{{if le (len .FieldSep) 1}}
		_fields = strings.Split(_record, {{printf "%q" .FieldSep}})
{{else}}
		if _fieldSepRegex == nil {
			_fieldSepRegex = regexp.MustCompile({{printf "%q" .FieldSep}})
		}
		_fields = _fieldSepRegex.Split(_record, -1)
{{end}}
{{end}}
}

func NF() int {
	_ensureFields()
	return len(_fields)
}

func Match(re, s string) bool {
	regex := _reCompile(re)
	return regex.MatchString(s)
}

func Replace(re, s, repl string) string {
	regex := _reCompile(re)
	return regex.ReplaceAllString(s, repl)
}

func Submatches(re, s string) []string {
	regex := _reCompile(re)
	return regex.FindStringSubmatch(s)
}

var _reCache = make(map[string]*regexp.Regexp)

func _reCompile(re string) *regexp.Regexp {
	if regex, ok := _reCache[re]; ok {
		return regex
	}
	regex, err := regexp.Compile(re)
	if err != nil {
		_errorf("invalid regex %q: %v", re, err)
	}
	// Dumb, non-LRU cache: just cache the first 100 regexes
	if len(_reCache) < 100 {
		_reCache[re] = regex
	}
	return regex
}

func Substr(s string, n int, ms ...int) string {
	var m int
	switch len(ms) {
	case 0:
		m = len(s)
	case 1:
		m = ms[0]
	default:
		_errorf("Substr() takes 2 or 3 arguments")
	}

	if n < 0 {
		n = len(s) + n
	}
	if n < 0 {
		n = 0
	}
	if n > len(s) {
		n = len(s)
	}

	if m < 0 {
		m = len(s) + m
	}
	if m < 0 {
		m = 0
	}
	if m > len(s) {
		m = len(s)
	}

	return s[n:m]
}

func _errorf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
`))
