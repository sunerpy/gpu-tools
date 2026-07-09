package report

import (
	"errors"
	"os"
	"reflect"
	"syscall"
	"testing"
	"text/tabwriter"
	_ "unsafe"
)

func Test_TableRenderer_Render_returns_writer_error_when_empty_snapshot_writer_fails(t *testing.T) {
	// Given
	writer := failingWriter{}

	// When
	err := TableRenderer{}.Render(writer, emptySnapshot())

	// Then
	if !errors.Is(err, errWriteFailed) {
		t.Fatalf("Render error = %v, want errWriteFailed", err)
	}
}

func Test_TableRenderer_Render_returns_tabwriter_error_when_header_write_fails(t *testing.T) {
	// Given
	restore := patchTabwriterWrite(t, 1)
	defer restore()

	// When
	err := TableRenderer{}.Render(discardWriter{}, fixedSnapshot())

	// Then
	if !errors.Is(err, errTabwriterWriteFailed) {
		t.Fatalf("Render error = %v, want errTabwriterWriteFailed", err)
	}
}

func Test_TableRenderer_Render_returns_tabwriter_error_when_device_write_fails(t *testing.T) {
	// Given
	restore := patchTabwriterWrite(t, 2)
	defer restore()

	// When
	err := TableRenderer{}.Render(discardWriter{}, fixedSnapshot())

	// Then
	if !errors.Is(err, errTabwriterWriteFailed) {
		t.Fatalf("Render error = %v, want errTabwriterWriteFailed", err)
	}
}

func Test_TableRenderer_Render_returns_flush_error_when_tabwriter_flush_fails(t *testing.T) {
	// Given
	restore := patchTabwriterFlush(t)
	defer restore()

	// When
	err := TableRenderer{}.Render(discardWriter{}, fixedSnapshot())

	// Then
	if !errors.Is(err, errTabwriterFlushFailed) {
		t.Fatalf("Render error = %v, want errTabwriterFlushFailed", err)
	}
}

func Test_TableRenderer_Render_returns_tabwriter_error_when_process_header_write_fails(t *testing.T) {
	// Given
	restore := patchTabwriterWrite(t, 4)
	defer restore()

	// When
	err := TableRenderer{}.Render(discardWriter{}, processSnapshot())

	// Then
	if !errors.Is(err, errTabwriterWriteFailed) {
		t.Fatalf("Render error = %v, want errTabwriterWriteFailed", err)
	}
}

func Test_TableRenderer_Render_returns_tabwriter_error_when_process_row_write_fails(t *testing.T) {
	// Given
	restore := patchTabwriterWrite(t, 5)
	defer restore()

	// When
	err := TableRenderer{}.Render(discardWriter{}, processSnapshot())

	// Then
	if !errors.Is(err, errTabwriterWriteFailed) {
		t.Fatalf("Render error = %v, want errTabwriterWriteFailed", err)
	}
}

var errTabwriterWriteFailed = errors.New("tabwriter write failed")

var errTabwriterFlushFailed = errors.New("tabwriter flush failed")

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

var patchedTabwriterWriteCalls int

var patchedTabwriterWriteFailOn int

func patchedTabwriterWrite(_ *tabwriter.Writer, p []byte) (int, error) {
	patchedTabwriterWriteCalls++
	if patchedTabwriterWriteCalls == patchedTabwriterWriteFailOn {
		return 0, errTabwriterWriteFailed
	}
	return len(p), nil
}

func patchedTabwriterFlush(_ *tabwriter.Writer) error {
	return errTabwriterFlushFailed
}

func patchTabwriterWrite(t *testing.T, failOn int) func() {
	t.Helper()

	patchedTabwriterWriteCalls = 0
	patchedTabwriterWriteFailOn = failOn
	return patchFunction(
		t,
		reflect.ValueOf((*tabwriter.Writer).Write).Pointer(),
		reflect.ValueOf(patchedTabwriterWrite).Pointer(),
	)
}

func patchTabwriterFlush(t *testing.T) func() {
	t.Helper()

	return patchFunction(
		t,
		reflect.ValueOf(tabwriterFlush).Pointer(),
		reflect.ValueOf(patchedTabwriterFlush).Pointer(),
	)
}

//go:linkname tabwriterFlush text/tabwriter.(*Writer).flush
func tabwriterFlush(*tabwriter.Writer) error

func patchFunction(t *testing.T, target, replacement uintptr) func() {
	t.Helper()

	const jumpSize = 12
	pageSize := syscall.Getpagesize()
	pageStart := target & ^uintptr(pageSize-1)
	setPageProtection(t, pageStart, pageSize, syscall.PROT_READ|syscall.PROT_WRITE|syscall.PROT_EXEC)

	original := readProcessMemory(t, target, jumpSize)
	writeProcessMemory(t, target, amd64Jump(replacement))

	return func() {
		writeProcessMemory(t, target, original)
		setPageProtection(t, pageStart, pageSize, syscall.PROT_READ|syscall.PROT_EXEC)
	}
}

func setPageProtection(t *testing.T, pageStart uintptr, pageSize, protection int) {
	t.Helper()

	_, _, errno := syscall.RawSyscall(syscall.SYS_MPROTECT, pageStart, uintptr(pageSize), uintptr(protection))
	if errno != 0 {
		t.Fatalf("mprotect function page: %v", errno)
	}
}

func readProcessMemory(t *testing.T, address uintptr, size int) []byte {
	t.Helper()

	mem, err := os.OpenFile("/proc/self/mem", os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open process memory: %v", err)
	}
	defer mem.Close()

	data := make([]byte, size)
	if _, err := mem.ReadAt(data, int64(address)); err != nil {
		t.Fatalf("read process memory: %v", err)
	}
	return data
}

func writeProcessMemory(t *testing.T, address uintptr, data []byte) {
	t.Helper()

	mem, err := os.OpenFile("/proc/self/mem", os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open process memory: %v", err)
	}
	defer mem.Close()

	if _, err := mem.WriteAt(data, int64(address)); err != nil {
		t.Fatalf("write process memory: %v", err)
	}
}

func amd64Jump(to uintptr) []byte {
	return []byte{
		0x48, 0xB8,
		byte(to), byte(to >> 8), byte(to >> 16), byte(to >> 24),
		byte(to >> 32), byte(to >> 40), byte(to >> 48), byte(to >> 56),
		0xFF, 0xE0,
	}
}
