package agentclient

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"fmt"
)

// readExeVersion reads the FileVersion from a Windows PE executable's
// VS_VERSION_INFO resource. Works on any OS by parsing the binary directly.
// Returns "" if no version can be determined.
func readExeVersion(exePath string) string {
	f, err := pe.Open(exePath)
	if err != nil {
		return ""
	}
	defer f.Close()

	// Find the .rsrc section — version resources live there.
	var rsrcData []byte
	for _, s := range f.Sections {
		if s.Name == ".rsrc" {
			rsrcData, err = s.Data()
			if err != nil {
				return ""
			}
			break
		}
	}
	if rsrcData == nil {
		return ""
	}

	return parseVersionFromRSRC(rsrcData)
}

// parseVersionFromRSRC searches for the VS_FIXEDFILEINFO magic (0xFEEF04BD) within
// a .rsrc section blob and extracts the FileVersion from the fixed info block.
func parseVersionFromRSRC(data []byte) string {
	// VS_FIXEDFILEINFO signature in little-endian
	magic := []byte{0xBD, 0x04, 0xEF, 0xFE}
	idx := bytes.Index(data, magic)
	if idx < 0 {
		return ""
	}

	// Layout after the magic:
	//   +0  dwSignature       (4 bytes) ← we're here
	//   +4  dwStrucVersion    (4 bytes)
	//   +8  dwFileVersionMS   (4 bytes) — major<<16 | minor
	//   +12 dwFileVersionLS   (4 bytes) — patch<<16 | build
	const fixedInfoSize = 4 + 4 + 4 + 4 // sig + strucVer + MS + LS
	if idx+fixedInfoSize > len(data) {
		return ""
	}

	ms := binary.LittleEndian.Uint32(data[idx+8:])
	ls := binary.LittleEndian.Uint32(data[idx+12:])

	major := ms >> 16
	minor := ms & 0xFFFF
	patch := ls >> 16
	build := ls & 0xFFFF

	if major == 0 && minor == 0 {
		return ""
	}
	if build > 0 {
		return fmt.Sprintf("%d.%d.%d.%d", major, minor, patch, build)
	}
	if patch > 0 {
		return fmt.Sprintf("%d.%d.%d", major, minor, patch)
	}
	return fmt.Sprintf("%d.%d", major, minor)
}
