package main

import (
	"io"
	"strings"
	"testing"
	"time"
)


func TestRotateArchiveFile(t *testing.T) {
	writerCalled := make(chan bool)

	broadcast := make(chan []byte, broadcastBuffSize)
	ts := time.Date(2015, time.September, 18, 23, 0, 0, 0, time.UTC)

	writer := func (info archiveInfo, openFile fileOpener) {
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


type FakeOpener struct {
	io.WriteCloser
}

func (f FakeOpener) Write(p []byte) (n int, err error) {
	return 0, nil
}

func (f FakeOpener) Close() error {
	return nil
}

func fakeOpen(name string) (io.WriteCloser, error) {
	return FakeOpener{}, nil
}


func TestWriteArchiveFile(t *testing.T) {
	broadcast := make(chan []byte)
	quit := make(chan int)

	go writeArchiveFile(
		archiveInfo{broadcast, quit, "some_file.mp3"},
		fakeOpen)

	// Send some data through the broadcast channel.
	broadcast <- []byte{0, 0}
	// TODO: figure out how to test that some output was written.
	close(quit)
}
