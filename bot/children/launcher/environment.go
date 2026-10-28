package launcher

import (
	"os"
	"strconv"
	"strings"
)

var inheritedVariableNames = []string{
	"PATH", "HOME", "USERPROFILE", "SYSTEMROOT", "TEMP", "TMP", "TMPDIR",
	"LOCALAPPDATA", "APPDATA", "LANG", "TZ",
	"GOROOT", "GOPATH", "GOCACHE", "GOMODCACHE", "GOPROXY", "GOSUMDB", "GOPRIVATE", "GONOSUMDB",
	"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "SSL_CERT_FILE", "SSL_CERT_DIR",
}

type ChildRedis struct {
	Address  string
	Password string
	Database int
}

func inheritedEnvironment() []string {
	selectedVariables := make([]string, 0, len(inheritedVariableNames))
	for _, environmentEntry := range os.Environ() {
		variableName, _, hasValue := strings.Cut(environmentEntry, "=")
		if !hasValue {
			continue
		}
		for _, allowedName := range inheritedVariableNames {
			if strings.EqualFold(variableName, allowedName) {
				selectedVariables = append(selectedVariables, environmentEntry)
				break
			}
		}
	}
	return selectedVariables
}

func buildEnvironment(hasModuleFile bool) []string {
	buildVariables := append(inheritedEnvironment(), "CGO_ENABLED=0", "GOTOOLCHAIN=local")
	if hasModuleFile {
		buildVariables = append(buildVariables, "GOFLAGS=-mod=mod")
	}
	return buildVariables
}

func childEnvironment(childBotToken string, childRedis ChildRedis) []string {
	return append(inheritedEnvironment(),
		"BOT_TOKEN="+childBotToken,
		"REDIS_ADDR="+childRedis.Address,
		"REDIS_PASSWORD="+childRedis.Password,
		"REDIS_DB="+strconv.Itoa(childRedis.Database),
	)
}
