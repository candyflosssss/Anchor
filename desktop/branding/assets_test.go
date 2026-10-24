package branding

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"os"
	"testing"
)

func TestWindowsApplicationIconResource(t *testing.T) {
	checkWindowsIcon(t, "../../cmd/anchor-desktop/rsrc_windows_amd64.syso")
}

func TestLinkedWindowsApplicationIcon(t *testing.T) {
	path := os.Getenv("ANCHOR_TEST_WINDOWS_EXE")
	if path == "" {
		t.Skip("set ANCHOR_TEST_WINDOWS_EXE to inspect a cross-compiled desktop executable")
	}
	checkWindowsIcon(t, path)
}

func checkWindowsIcon(t *testing.T, path string) {
	t.Helper()
	f, err := pe.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if f.Machine != pe.IMAGE_FILE_MACHINE_AMD64 {
		t.Fatal("Windows desktop resource must target AMD64")
	}
	section := f.Section(".rsrc")
	if section == nil {
		t.Fatal("Windows desktop has no native resource table")
	}
	raw, err := section.Data()
	if err != nil {
		t.Fatal(err)
	}
	// COFF offsets are section-relative; the PE linker relocates data to RVAs.
	resource := func(kind, id uint32) []byte {
		entry := resourceEntry(t, raw, 0, kind)
		entry = resourceEntry(t, raw, entry, id)
		entry = resourceEntry(t, raw, entry, 0) // neutral language
		data := resourceBytes(t, raw, entry, 16)
		offset := binary.LittleEndian.Uint32(data) - section.VirtualAddress
		return resourceBytes(t, raw, offset, binary.LittleEndian.Uint32(data[4:]))
	}
	// Wails v2 loads RT_GROUP_ICON (14), ID 3. A tray-only embedded ICO
	// or a resource under another ID cannot supply the main window's icon.
	group := resource(14, 3)
	if len(group) < 6 || !bytes.Equal(group[:6], WindowsIcon[:6]) {
		t.Fatal("native icon group does not match anchor.ico")
	}
	count := int(binary.LittleEndian.Uint16(group[4:]))
	if len(group) != 6+14*count {
		t.Fatal("invalid native icon group size")
	}
	originals := make(map[string][]byte, count)
	for i := range count {
		icon := WindowsIcon[6+16*i : 6+16*(i+1)]
		originals[string(icon[:12])] = resourceBytes(t, WindowsIcon, binary.LittleEndian.Uint32(icon[12:]), binary.LittleEndian.Uint32(icon[8:]))
	}
	// The compiler may reorder image sizes when forming the native group.
	for i := range count {
		compiled := group[6+14*i : 6+14*(i+1)]
		original, ok := originals[string(compiled[:12])]
		if !ok {
			t.Fatalf("native icon size/depth missing from anchor.ico at image %d", i)
		}
		data := resource(3, uint32(binary.LittleEndian.Uint16(compiled[12:])))
		if !bytes.Equal(data, original) {
			t.Fatalf("native icon image %d is stale; run go generate ./desktop/branding", i)
		}
		delete(originals, string(compiled[:12]))
	}
}

func resourceEntry(t *testing.T, raw []byte, offset, id uint32) uint32 {
	t.Helper()
	header := resourceBytes(t, raw, offset, 16)
	named := uint32(binary.LittleEndian.Uint16(header[12:]))
	count := uint32(binary.LittleEndian.Uint16(header[14:]))
	for i := range count {
		entry := resourceBytes(t, raw, offset+16+8*(named+i), 8)
		if binary.LittleEndian.Uint32(entry) == id {
			return binary.LittleEndian.Uint32(entry[4:]) & 0x7fffffff
		}
	}
	t.Fatalf("resource ID %d missing from directory at %d", id, offset)
	return 0
}

func resourceBytes(t *testing.T, raw []byte, offset, size uint32) []byte {
	t.Helper()
	if uint64(offset)+uint64(size) > uint64(len(raw)) {
		t.Fatal("resource points outside its section")
	}
	return raw[offset : offset+size]
}
