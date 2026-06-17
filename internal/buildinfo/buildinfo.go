package buildinfo

import (
	"encoding/json"
	"fmt"
	"runtime"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
	GOOS      string `json:"goos"`
	GOARCH    string `json:"goarch"`
}

func Current() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
		GoVersion: runtime.Version(),
		GOOS:      runtime.GOOS,
		GOARCH:    runtime.GOARCH,
	}
}

func Text() string {
	info := Current()
	return fmt.Sprintf("smart-llmrouter version=%s commit=%s build_date=%s go=%s %s/%s", info.Version, info.Commit, info.BuildDate, info.GoVersion, info.GOOS, info.GOARCH)
}

func JSON() string {
	raw, err := json.MarshalIndent(Current(), "", "  ")
	if err != nil {
		return "{}"
	}
	return string(raw)
}
