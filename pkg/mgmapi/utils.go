package mgmapi

import "fmt"

// getMySQLVersionString extracts the MySQL Version of
// format major.minor.build from the given versionNumber
func getMySQLVersionString(versionNumber int) string {
	if versionNumber == 0 {
		return ""
	}

	major := (versionNumber >> 16) & 0xFF
	minor := (versionNumber >> 8) & 0xFF
	build := (versionNumber >> 0) & 0xFF

	return fmt.Sprintf("%d.%d.%d", major, minor, build)
}
