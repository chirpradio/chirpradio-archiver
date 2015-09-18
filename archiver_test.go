package main

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)


func TestRotateArchiveFile(t *testing.T) {
	writerCalled := make(chan bool)

	broadcast := make(chan []byte, broadcastBuffSize)
	ts := time.Date(2015, time.September, 18, 23, 0, 0, 0, time.UTC)

	writer := func (info archiveInfo, openFile fileOpener) error {
		fileIsOk := strings.HasSuffix(info.fileName,
			"chirpradio_2015-09-18_230000.mp3")
		if !fileIsOk {
			t.Error("Unexpected filename:", info.fileName)
		}
		writerCalled <- true
		close(writerCalled)
		return nil
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


func TestWriteArchiveFile(t *testing.T) {
	broadcast := make(chan []byte)
	quit := make(chan int)

	openFakeFile := func(name string) (io.WriteCloser, error) {
		return FakeOpener{}, nil
	}

	go writeArchiveFile(
		archiveInfo{broadcast, quit, "some_file.mp3"},
		openFakeFile)

	// Send some data through the broadcast channel.
	broadcast <- []byte{0, 0}
	// TODO: figure out how to test that some output was written.
	close(quit)
}


func TestWriteArchiveFileWithError(t *testing.T) {
	broadcast := make(chan []byte)
	quit := make(chan int)
	// Close the channel in case the implementation doesn't return early as
	// expected.
	close(quit)

	openFileAndReturnError := func(name string) (io.WriteCloser, error) {
		return FakeOpener{}, errors.New("some error")
	}

	result := writeArchiveFile(
		archiveInfo{broadcast, quit, "some_file.mp3"},
		openFileAndReturnError)

	if result == nil {
		t.Error("Unexpected result")
	}
}
