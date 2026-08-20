package version

// These values are replaced at release time with -ldflags -X.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
}

func Current() Info { return Info{Version: Version, Commit: Commit, BuildDate: BuildDate} }
