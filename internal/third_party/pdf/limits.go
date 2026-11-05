package pdf

// simulator-ai addition: what reading one file may cost. Every stream read goes through a reader
// that counts the bytes each filter layer produces against one budget and checks the deadline, so
// a filter bomb (a chain of FlateDecode layers, names written in hex, an indirect /Filter, a huge
// dictionary: whatever the dictionary says) stops at Bytes; past either limit the read panics
// with ErrLimit, which the reader's callers recover.

import (
	"errors"
	"io"
	"time"
)

// ErrLimit is the panic value of a read past the byte budget or the deadline.
var ErrLimit = errors.New("pdf: read limit reached")

// Limits is the budget of one file: the bytes all filter layers may produce together, and the time.
type Limits struct {
	Bytes    int64
	Deadline time.Time
}

const (
	maxXrefEntries   = 1 << 20 // a cross-reference table larger than this is refused
	maxPredictorCols = 1 << 16
	maxXrefWidth     = 8
)

// NewReaderLimited is NewReader within limits: every stream read, from the cross-reference
// stream on, counts against them.
func NewReaderLimited(f io.ReaderAt, size int64, limits *Limits) (*Reader, error) {
	return newReader(f, size, nil, limits)
}

// check panics with ErrLimit once the deadline has passed.
func (l *Limits) check() {
	if l != nil && !l.Deadline.IsZero() && time.Now().After(l.Deadline) {
		panic(ErrLimit)
	}
}

// wrap counts what rd produces against the budget.
func (l *Limits) wrap(rd io.Reader) io.Reader {
	if l == nil {
		return rd
	}
	return &limitedReader{rd: rd, l: l}
}

type limitedReader struct {
	rd io.Reader
	l  *Limits
}

func (r *limitedReader) Read(p []byte) (int, error) {
	r.l.check()
	if r.l.Bytes <= 0 {
		panic(ErrLimit)
	}
	if int64(len(p)) > r.l.Bytes {
		p = p[:r.l.Bytes]
	}
	n, err := r.rd.Read(p)
	r.l.Bytes -= int64(n)
	return n, err
}
