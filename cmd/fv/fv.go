// Binary fv uses measuringringbuffer to show how much time is spent reading vs writing. It can be used instead of pv(1).
package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/Jille/measuringringbuffer"
	"github.com/dustin/go-humanize"
	"github.com/spf13/pflag"
)

var (
	size = pflag.IntP("size", "s", 8, "Number of megabytes of buffer")

	printMtx    sync.Mutex
	widths      [6]int
	parts       [6]string
	printBuffer bytes.Buffer
)

func main() {
	pflag.Parse()

	buf := measuringringbuffer.New(*size * 1024 * 1024)
	go func() {
		for range time.Tick(time.Second / 2) {
			printMtx.Lock()
			err := printStats(buf.Stats(), '\r')
			printMtx.Unlock()
			if err != nil {
				return
			}
		}
	}()
	_, err := buf.Copy(os.Stdout, os.Stdin)
	printMtx.Lock()
	printStats(buf.Stats(), '\n')
	if err != nil {
		log.Fatal(err)
	}
}

func printStats(s measuringringbuffer.Stats, lineEnd byte) error {
	parts[0] = fmt.Sprintf("Buffer: %s/%s", humanize.Bytes(uint64(s.BufferedBytes)), humanize.Bytes(uint64(s.BufferCapacity)))
	parts[1] = s.TotalTime.Truncate(time.Second).String()
	parts[2] = fmt.Sprintf("B: %s", humanize.Bytes(uint64(s.BytesRead)))
	parts[3] = fmt.Sprintf("S: %s", humanize.SIWithDigits(float64(s.BytesRead)/s.TotalTime.Seconds(), 2, "B/s"))
	parts[4] = fmt.Sprintf("R: % 2d%%", int(100*s.TimeSpentReading/s.TotalTime))
	parts[5] = fmt.Sprintf("W: % 2d%%", int(100*s.TimeSpentWriting/s.TotalTime))
	for i, p := range parts {
		if len(p) > widths[i] {
			widths[i] = len(p)
		}
		printBuffer.WriteString(p)
		if i < len(parts)-1 {
			padding := widths[i] + 1 - len(p)
			printBuffer.Write(bytes.Repeat([]byte{' '}, padding))
		}
	}
	printBuffer.WriteByte(lineEnd)
	_, err := io.Copy(os.Stderr, &printBuffer)
	printBuffer.Reset()
	return err
}
