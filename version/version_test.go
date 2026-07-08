package version

import "testing"

func Test_Info_and_Short_format_build_metadata(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		buildTime string
		commitID  string
		buildOS   string
		buildArch string
		wantInfo  string
		wantShort string
	}{
		{
			name:      "default values",
			version:   "dev",
			buildTime: "dev",
			commitID:  "dev",
			buildOS:   "dev",
			buildArch: "dev",
			wantInfo:  "Version: dev\nBuildTime: dev\nCommitID: dev\nBuildOS: dev\nBuildArch: dev",
			wantShort: "dev",
		},
		{
			name:      "injected values",
			version:   "v1.2.3",
			buildTime: "2026-07-08T12:34:56Z",
			commitID:  "abc1234",
			buildOS:   "linux",
			buildArch: "amd64",
			wantInfo:  "Version: v1.2.3\nBuildTime: 2026-07-08T12:34:56Z\nCommitID: abc1234\nBuildOS: linux\nBuildArch: amd64",
			wantShort: "v1.2.3",
		},
		{
			name:      "empty version falls back",
			version:   "",
			buildTime: "2026-07-08T12:34:56Z",
			commitID:  "abc1234",
			buildOS:   "linux",
			buildArch: "arm64",
			wantInfo:  "Version: \nBuildTime: 2026-07-08T12:34:56Z\nCommitID: abc1234\nBuildOS: linux\nBuildArch: arm64",
			wantShort: "dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			restoreVersionVars(t)
			Version = tt.version
			BuildTime = tt.buildTime
			CommitID = tt.commitID
			BuildOS = tt.buildOS
			BuildArch = tt.buildArch

			// When
			gotInfo := Info()
			gotShort := Short()

			// Then
			if gotInfo != tt.wantInfo {
				t.Fatalf("Info() = %q, want %q", gotInfo, tt.wantInfo)
			}
			if gotShort != tt.wantShort {
				t.Fatalf("Short() = %q, want %q", gotShort, tt.wantShort)
			}
		})
	}
}

func restoreVersionVars(t *testing.T) {
	t.Helper()

	originalVersion := Version
	originalBuildTime := BuildTime
	originalCommitID := CommitID
	originalBuildOS := BuildOS
	originalBuildArch := BuildArch

	t.Cleanup(func() {
		Version = originalVersion
		BuildTime = originalBuildTime
		CommitID = originalCommitID
		BuildOS = originalBuildOS
		BuildArch = originalBuildArch
	})
}
