# measuringringbuffer

[![GoDoc](https://godoc.org/github.com/Jille/measuringringbuffer?status.svg)](https://godoc.org/github.com/Jille/measuringringbuffer)

This is a binary and library implementing a ring buffer of bytes that measures how much time is spend reading vs writing. The binary, fv (flowviewer) can be used instead of pv(1) to give more insight into where your bottleneck lies.

The output of the fv binary is expected to change in the future.
