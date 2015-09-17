package main

import (
	"strings"
	"testing"
	"time"
)


func TestRotateArchiveFile(t *testing.T) {
	writerCalled := make(chan bool)

	broadcast := make(chan []byte, broadcastBuffSize)
	ts := time.Date(2015, time.September, 18, 23, 0, 0, 0, time.UTC)

	writer := func (info archiveInfo) {
		fileIsOk := strings.HasSuffix(info.fileName,
			"chirpradio_2015-09-18_230000.mp3")
		if !fileIsOk {
			t.Error("Unexpected filename:", info.fileName)
		}
		writerCalled <- true
		close(writerCalled)
	}

	rotateArchiveFile(broadcast, ts, writer)

	for {
		select {
		case <-writerCalled:
			return
		case <-time.After(3 * time.Second):
			panic("timeout: rotateArchiveFile should call writer()")
		}
	}
}
