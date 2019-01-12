package openvcdiff

// #cgo LDFLAGS: -L${SRCDIR}/../../../open-vcdiff/build -lvcdenc -lvcdcom
// #cgo CPPFLAGS: -I${SRCDIR}/../../../open-vcdiff/src
// #include "exports.h"
// #include <stdlib.h>
import "C"

import (
	"errors"
	"io"
	"sync"
	"unsafe"
)

type FormatFlags int

const (
	VCDStandardFormat    FormatFlags = 0x00
	VCDFormatInterleaved FormatFlags = 0x01
	VCDFormatChecksum    FormatFlags = 0x02
	VCDFormatJSON        FormatFlags = 0x04

	VCDLookForTargetMatches FormatFlags = 0xff010000
)

type Dictionary struct {
	ptr C.HashedDictionaryPtr
}

func NewDictionary(dict []byte) (*Dictionary, error) {
	dictPtr := C.CBytes(dict)
	defer C.free(dictPtr)

	ptr := C.NewHashedDictionary((*C.char)(dictPtr), C.ulong(len(dict)))
	if ptr == nil {
		return nil, errors.New("open-vcdiff: failed to initialise dictionary")
	}

	return &Dictionary{ptr}, nil
}

func (d *Dictionary) cPtr() C.HashedDictionaryPtr {
	if d.ptr == nil {
		panic("open-vcdiff: cannot use Dictionary after Destroy")
	}

	return d.ptr
}

func (d *Dictionary) Destroy() {
	if d.ptr != nil {
		C.DeleteHashedDictionary(d.ptr)
		d.ptr = nil
	}
}

type Encoder struct {
	ptr C.VCDiffStreamingEncoderPtr

	writerIdx int
}

func NewEncoder(w io.Writer, dict *Dictionary, flags FormatFlags) (*Encoder, error) {
	writerIdx := writers.insert(w)

	ptr := C.NewVCDiffStreamingEncoder(dict.cPtr(),
		C.int(flags&^VCDLookForTargetMatches),
		C.int(flags&VCDLookForTargetMatches),
		C.int(writerIdx))
	if ptr == nil {
		writers.delete(writerIdx)
		return nil, errors.New("open-vcdiff: failed to start encoder")
	}

	return &Encoder{ptr, writerIdx}, nil
}

func (e *Encoder) Write(p []byte) (int, error) {
	if e.ptr == nil {
		return 0, errors.New("open-vcdiff: cannot write to Encoder after Close")
	}

	ptr := C.CBytes(p)
	defer C.free(ptr)

	w := writers.find(e.writerIdx)
	if w.err != nil {
		return 0, w.err
	}

	if C.VCDiffStreamingEncoderEncodeChunk(e.ptr, C.int(e.writerIdx), (*C.char)(ptr), C.ulong(len(p))) != 1 {
		return 0, errors.New("open-vcdiff: failed to encode chunk")
	}

	return len(p), w.err
}

func (e *Encoder) Close() error {
	if e.ptr == nil {
		return nil
	}

	ok := C.VCDiffStreamingEncoderFinishEncoding(e.ptr, C.int(e.writerIdx))
	e.ptr = nil

	w := writers.delete(e.writerIdx)

	if ok != 1 {
		return errors.New("open-vcdiff: failed to finish encoding")
	}

	return w.err
}

type writer struct {
	io.Writer
	err error
}

type writersMap struct {
	mu  sync.RWMutex
	m   map[int]*writer
	idx int
}

func (m *writersMap) insert(w io.Writer) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	for tries := 0; ; tries++ {
		idx := m.idx
		m.idx++

		if _, ok := writers.m[idx]; !ok {
			m.m[idx] = &writer{Writer: w}
			return idx
		}
		if tries > 100 {
			panic("open-vcdiff: cannot find free slot in writersMap")
		}
	}
}

func (m *writersMap) find(idx int) *writer {
	m.mu.RLock()
	w := m.m[idx]
	m.mu.RUnlock()
	return w
}

func (m *writersMap) delete(idx int) *writer {
	m.mu.Lock()
	w := m.m[idx]
	delete(m.m, idx)
	m.mu.Unlock()
	return w
}

var writers = writersMap{m: make(map[int]*writer)}

//export goOpenVCDIFFWriterWrite
func goOpenVCDIFFWriterWrite(writerIdx int, p *C.char, n C.ulong) {
	w := writers.find(writerIdx)
	if w.err != nil {
		return
	}

	_, w.err = w.Write(C.GoBytes(unsafe.Pointer(p), C.int(n)))
}
