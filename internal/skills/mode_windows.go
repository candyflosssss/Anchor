package skills

import "os"

// NTFS does not preserve POSIX execute bits. Snapshots use a stable false bit;
// executable extensions and the Agent's interpreter remain authoritative.
func executable(os.FileMode) bool { return false }
func executableFlag(bool) bool    { return false }
