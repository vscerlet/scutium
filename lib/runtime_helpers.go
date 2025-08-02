package lib

import (
	"regexp"
	"runtime"
	"strings"
)

func GetCurrentFuncName() string {
	// Get the program counter, file, line, and ok flag for the caller at depth 1
	pc, _, _, ok := runtime.Caller(1)

	// Retrieve function details for the given program counter
	details := runtime.FuncForPC(pc)

	if ok && details != nil {
		// Get the full function name, e.g. "package.(*Type).Method"
		name := details.Name()

		// Strip the path, keep only the package and function part after the last slash
		lastSlash := strings.LastIndex(name, "/")
		if lastSlash >= 0 {
			name = name[lastSlash+1:]
		}

		// regexp to match method receivers in form "package.(*Type).Method"
		// and extract package and method name parts
		re := regexp.MustCompile(`^([^.]+)\.\([^)]+\)\.(.+)$`)

		// If regex matches, return "package.Method" format
		if matches := re.FindStringSubmatch(name); len(matches) == 3 {
			return matches[1] + "." + matches[2] // package.method
		}

		// Otherwise, return the raw function name
		return name
	}

	// Return empty string if caller info could not be retrieved
	return ""
}
