package version

import "fmt"

const fallbackVersion = "dev"

var (
	Version   string = "dev"
	BuildTime string = "dev"
	CommitID  string = "dev"
	BuildOS   string = "dev"
	BuildArch string = "dev"
)

func Short() string {
	if Version == "" {
		return fallbackVersion
	}

	return Version
}

func Info() string {
	return fmt.Sprintf(
		"Version: %s\nBuildTime: %s\nCommitID: %s\nBuildOS: %s\nBuildArch: %s",
		Version,
		BuildTime,
		CommitID,
		BuildOS,
		BuildArch,
	)
}
