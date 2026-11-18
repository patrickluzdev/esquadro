package workspace

import "golang.org/x/mod/semver"

// Versions is how the esquadro running compares with the one that last
// generated a project.
type Versions int

const (
	// SameVersion: generate as usual.
	SameVersion Versions = iota
	// NewerVersion: the project is from an older release; esquadro upgrade
	// shows what changed, then generates.
	NewerVersion
	// OlderVersion: the project is from a newer release; this esquadro
	// would undo what it brought.
	OlderVersion
	// DevelopmentVersion: one of them is no release (a build of esquadro's
	// own code): they may differ, and generating goes on.
	DevelopmentVersion
)

// CompareVersions compares running, the esquadro generating now, with
// project, the one in the project's lock ("" for a project never
// generated).
func CompareVersions(project, running string) Versions {
	switch {
	case project == "" || project == running:
		return SameVersion
	case !Released(project) || !Released(running):
		return DevelopmentVersion
	}
	switch semver.Compare(running, project) {
	case 1:
		return NewerVersion
	case -1:
		return OlderVersion
	}
	return SameVersion
}

// Released reports whether v is a release: v1.2.3, not a pseudo-version, a
// pre-release or a development build.
func Released(v string) bool {
	return semver.IsValid(v) && semver.Prerelease(v) == "" && semver.Build(v) == ""
}
