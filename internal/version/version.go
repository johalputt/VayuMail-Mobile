// Package version is the single source of the application version. Bump
// both values together: Code is the monotonically increasing integer
// Android/iOS use, Semantic is what humans and gogio's -version flag use.
package version

const (
	// Semantic is the human-readable release version.
	Semantic = "2.2.2"
	// Code is the platform version code (increments every release).
	Code = 31
)
