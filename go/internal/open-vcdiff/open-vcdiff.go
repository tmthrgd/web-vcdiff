package openvcdiff

// #cgo LDFLAGS: -L${SRCDIR}/../../../build/open-vcdiff -lvcddec -lvcdenc -lvcdcom
// #cgo CPPFLAGS: -I${SRCDIR}/../../../open-vcdiff/src
// #include "exports.h"
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
	var dictPtr *byte
	if len(dict) > 0 {
		dictPtr = &dict[0]
	}

	ptr := C.NewHashedDictionary((*C.char)(unsafe.Pointer(dictPtr)), C.ulong(len(dict)))
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

	w *writer
}

func NewEncoder(w io.Writer, dict *Dictionary, flags FormatFlags) (*Encoder, error) {
	ww := writers.insert(w)

	ptr := C.NewVCDiffStreamingEncoder(dict.cPtr(),
		C.int(flags&^VCDLookForTargetMatches),
		C.int(flags&VCDLookForTargetMatches),
		C.int(ww.idx))
	if ptr == nil {
		writers.delete(ww)
		return nil, errors.New("open-vcdiff: failed to start encoder")
	}

	return &Encoder{ptr, ww}, nil
}

func (e *Encoder) Write(p []byte) (int, error) {
	if e.ptr == nil {
		return 0, errors.New("open-vcdiff: cannot write to Encoder after Close")
	}
	if e.w.err != nil {
		return 0, e.w.err
	}
	if len(p) == 0 {
		// We want to return an error, even for a zero-length Write.
		return 0, nil
	}

	if C.VCDiffStreamingEncoderEncodeChunk(e.ptr, C.int(e.w.idx),
		(*C.char)(unsafe.Pointer(&p[0])), C.ulong(len(p))) != 1 {
		return 0, errors.New("open-vcdiff: failed to encode chunk")
	}

	return len(p), e.w.err
}

func (e *Encoder) Close() error {
	if e.ptr == nil {
		return nil
	}

	ok := C.VCDiffStreamingEncoderFinishEncoding(e.ptr, C.int(e.w.idx))
	e.ptr = nil

	writers.delete(e.w)

	if ok != 1 {
		return errors.New("open-vcdiff: failed to finish encoding")
	}

	return e.w.err
}

type Decoder struct {
	ptr C.VCDiffStreamingDecoderPtr

	w *writer
}

func NewDecoder(w io.Writer, dict []byte) (*Decoder, error) {
	var dictPtr *byte
	if len(dict) > 0 {
		dictPtr = &dict[0]
	}

	ptr := C.NewVCDiffStreamingDecoder((*C.char)(unsafe.Pointer(dictPtr)), C.ulong(len(dict)))
	if ptr == nil {
		return nil, errors.New("open-vcdiff: failed to start decoder")
	}

	return &Decoder{ptr, writers.insert(w)}, nil
}

func (d *Decoder) Write(p []byte) (int, error) {
	if d.ptr == nil {
		return 0, errors.New("open-vcdiff: cannot write to Decoder after Close")
	}
	if d.w.err != nil {
		return 0, d.w.err
	}
	if len(p) == 0 {
		// We want to return an error, even for a zero-length Write.
		return 0, nil
	}

	if C.VCDiffStreamingDecoderDecodeChunk(d.ptr, C.int(d.w.idx),
		(*C.char)(unsafe.Pointer(&p[0])), C.ulong(len(p))) != 1 {
		d.reset()
		return 0, errors.New("open-vcdiff: failed to decode chunk")
	}

	return len(p), d.w.err
}

func (d *Decoder) Close() error {
	if d.ptr == nil {
		return nil
	}

	ok := C.VCDiffStreamingDecoderFinishDecoding(d.ptr)
	d.reset()

	if ok != 1 {
		return errors.New("open-vcdiff: failed to finish decoding")
	}

	return d.w.err
}

func (d *Decoder) reset() {
	d.ptr = nil
	writers.delete(d.w)
}

type writer struct {
	io.Writer
	err error

	idx int
}

type writersMap struct {
	mu sync.RWMutex
	m  map[int]*writer

	idx int
}

func (m *writersMap) insert(w io.Writer) *writer {
	m.mu.Lock()
	defer m.mu.Unlock()
	for tries := 0; ; tries++ {
		idx := m.idx
		m.idx++

		if _, ok := m.m[idx]; !ok {
			w := &writer{w, nil, idx}
			m.m[idx] = w
			return w
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

func (m *writersMap) delete(w *writer) {
	m.mu.Lock()
	delete(m.m, w.idx)
	m.mu.Unlock()
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
