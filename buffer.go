// Package measuringringbuffer copies data from a reader to a writer keeping track of how slow both are.
//
// Each buffer should be used only once (one Copy or ReadFrom+WriteTo call). Most users should use the Copy API.
package measuringringbuffer

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// Buffer is a ring buffer that tracks stats about how much time is spent on reading vs writing.
type Buffer struct {
	buf              []byte
	mtx              sync.Mutex
	cond             *sync.Cond
	readIndex        int
	ready            int
	totalBytesRead   int64
	readError        error
	writeError       error
	timeStarted      time.Time
	lastReadStarted  time.Time
	lastWriteStarted time.Time
	timeSpentReading time.Duration
	timeSpentWriting time.Duration
	totalTimeSpent   time.Duration
}

var _ io.WriterTo = &Buffer{}
var _ io.ReaderFrom = &Buffer{}

func New(size int) *Buffer {
	b := &Buffer{
		buf: make([]byte, size),
	}
	b.cond = sync.NewCond(&b.mtx)
	return b
}

// Copy data from the reader to the writer.
func (b *Buffer) Copy(w io.Writer, r io.Reader) (int64, error) {
	go b.ReadFrom(r)
	return b.WriteTo(w)
}

// Stats are statistics of an active Buffer.
type Stats struct {
	BufferCapacity   int
	BufferedBytes    int
	BytesRead        int64
	TotalTime        time.Duration
	TimeSpentReading time.Duration
	TimeSpentWriting time.Duration
}

func (b *Buffer) Stats() Stats {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	now := time.Now()
	s := Stats{
		BufferCapacity:   len(b.buf),
		BufferedBytes:    b.ready,
		TimeSpentReading: b.timeSpentReading,
		TimeSpentWriting: b.timeSpentWriting,
		TotalTime:        b.totalTimeSpent,
		BytesRead:        b.totalBytesRead,
	}
	if !b.timeStarted.IsZero() {
		s.TotalTime += now.Sub(b.timeStarted)
	}
	if !b.lastReadStarted.IsZero() {
		s.TimeSpentReading += now.Sub(b.lastReadStarted)
	}
	if !b.lastWriteStarted.IsZero() {
		s.TimeSpentWriting += now.Sub(b.lastWriteStarted)
	}
	return s
}

func (b *Buffer) ReadFrom(r io.Reader) (int64, error) {
	var sum int64
	b.mtx.Lock()
	defer b.mtx.Unlock()
	if b.timeStarted.IsZero() {
		b.timeStarted = time.Now()
	}

	for {
		var until int
		if b.readIndex > b.ready {
			until = len(b.buf)
		} else {
			until = b.readIndex - b.ready + len(b.buf)
		}
		buf := b.buf[b.readIndex:until]
		b.lastReadStarted = time.Now()
		b.mtx.Unlock()

		n, err := r.Read(buf)
		dur := time.Since(b.lastReadStarted)
		sum += int64(n)

		b.mtx.Lock()
		b.readIndex = (b.readIndex + n) % len(b.buf)
		b.ready += n
		b.totalBytesRead += int64(n)
		b.timeSpentReading += dur
		b.lastReadStarted = time.Time{}
		b.cond.Signal()
		if err != nil {
			b.readError = err
			if err == io.EOF {
				return sum, nil
			}
			return sum, err
		}
		if b.writeError != nil {
			return sum, b.writeError
		}
		for b.ready == len(b.buf) {
			b.cond.Wait()
			if b.writeError != nil {
				return sum, b.writeError
			}
		}
	}
}

func (b *Buffer) WriteTo(w io.Writer) (int64, error) {
	// io.Writers block until all bytes are written. So we want to do smaller writes than possible to free up space for the reader sooner.
	maxWrite := len(b.buf) / 8
	if maxWrite < 1 {
		// Let's hope nobody actually uses this with a buffer less than 8 bytes..
		maxWrite = 1
	}
	var sum int64
	b.mtx.Lock()
	defer b.mtx.Unlock()
	if b.timeStarted.IsZero() {
		b.timeStarted = time.Now()
	}
	defer func() {
		// Stop the clock on the total timer.
		b.totalTimeSpent += time.Since(b.timeStarted)
		b.timeStarted = time.Time{}
	}()
	for {
		for b.ready == 0 {
			if b.readError != nil {
				if b.readError == io.EOF {
					return sum, nil
				}
				return sum, fmt.Errorf("read error: %w", b.readError)
			}
			b.cond.Wait()
		}
		var buf []byte
		if b.readIndex-b.ready >= 0 {
			buf = b.buf[b.readIndex-b.ready : b.readIndex]
		} else {
			buf = b.buf[b.readIndex-b.ready+len(b.buf):]
		}
		b.lastWriteStarted = time.Now()
		b.mtx.Unlock()

		if len(buf) > maxWrite {
			buf = buf[:maxWrite]
		}
		n, err := w.Write(buf)
		dur := time.Since(b.lastWriteStarted)
		sum += int64(n)

		b.mtx.Lock()
		b.ready -= n
		b.lastWriteStarted = time.Time{}
		b.timeSpentWriting += dur
		b.cond.Signal()
		if err != nil {
			err = fmt.Errorf("write error: %w", err)
			b.writeError = err
			return sum, err
		}
	}
}
