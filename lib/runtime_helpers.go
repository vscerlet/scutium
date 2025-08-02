package lib

import (
	"regexp"
	"runtime"
	"strings"
)

func GetCurrentFuncName() string {
	pc, _, _, ok := runtime.Caller(1)
	details := runtime.FuncForPC(pc)
	if ok && details != nil {
		name := details.Name()

		lastSlash := strings.LastIndex(name, "/")
		if lastSlash >= 0 {
			name = name[lastSlash+1:]
		}

		re := regexp.MustCompile(`^([^.]+)\.\([^)]+\)\.(.+)$`)
		if matches := re.FindStringSubmatch(name); len(matches) == 3 {
			return matches[1] + "." + matches[2] // пакет.метод
		}

		return name
	}
	return ""
}
