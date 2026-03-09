package agentclient

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"fmt"
)

// readExeVersion reads the ProductVersion from a Windows PE executable's
// VS_VERSION_INFO resource. Works on any OS by parsing the binary directly.
// ProductVersion is preferred over FileVersion because many engines (LÖVE, Unity, etc.)
// set FileVersion to the engine/framework version rather than the game version.
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
// a .rsrc section blob and extracts the ProductVersion from the fixed info block.
//
// VS_FIXEDFILEINFO layout (all DWORD = 4 bytes, little-endian):
//
//	+0  dwSignature       0xFEEF04BD
//	+4  dwStrucVersion
//	+8  dwFileVersionMS
//	+12 dwFileVersionLS
//	+16 dwProductVersionMS   major<<16 | minor
//	+20 dwProductVersionLS   patch<<16 | build
func parseVersionFromRSRC(data []byte) string {
	// VS_FIXEDFILEINFO signature in little-endian
	magic := []byte{0xBD, 0x04, 0xEF, 0xFE}
	idx := bytes.Index(data, magic)
	if idx < 0 || idx+24 > len(data) {
		return ""
	}

	productMS := binary.LittleEndian.Uint32(data[idx+16:])
	productLS := binary.LittleEndian.Uint32(data[idx+20:])

	major := productMS >> 16
	minor := productMS & 0xFFFF
	patch := productLS >> 16
	build := productLS & 0xFFFF

	// Fall back to FileVersion if ProductVersion is all zeros
	if major == 0 && minor == 0 {
		fileMS := binary.LittleEndian.Uint32(data[idx+8:])
		fileLS := binary.LittleEndian.Uint32(data[idx+12:])
		major = fileMS >> 16
		minor = fileMS & 0xFFFF
		patch = fileLS >> 16
		build = fileLS & 0xFFFF
	}

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
