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


type FakeStream struct {
	io.ReadCloser
}

func (FakeStream) Read(p []byte) (n int, err error) {
	return 0, nil
}

func (FakeStream) Close() error {
	return nil
}

func TestStreamBroadcast(t *testing.T) {
	urlOpened := make(chan bool)

	fakeUrlOpen := func(url string) (*MinimalHttpResponse, error) {
		urlOpened <- true
		return &MinimalHttpResponse{&FakeStream{}}, nil
	}

	session := NewBroadcastSession(fakeUrlOpen, 1)
	go streamBroadcast(session)

	// TODO: figure out how to test that the stream gets sent to the broadcast
	// channel.

	for {
		select {
		case <-urlOpened:
			close(session.quit)
			return
		case <-time.After(3 * time.Second):
			panic("timeout: streamBroadcast should open a URL")
		}
	}
}


func TestStreamBroadcastRetriesAfterOpenError(t *testing.T) {
	urlOpenCount := make(chan int)
	var counter int = 0;

	fakeUrlOpen := func(url string) (*MinimalHttpResponse, error) {
		counter += 1
		urlOpenCount <- counter
		return &MinimalHttpResponse{&FakeStream{}}, errors.New("some error")
	}

	session := NewBroadcastSession(fakeUrlOpen, 2)
	session.retrySleepTime = 1 * time.Nanosecond
	go streamBroadcast(session)

	for {
		select {
		case timesOpened := <-urlOpenCount:
			if timesOpened < 2 {
				log("stream not opened enough times:", timesOpened)
				continue
			}
			close(session.quit)
			return
		case <-time.After(3 * time.Second):
			panic("timeout: did not retry enough times after error")
		}
	}
}


type FakeErrorStream struct {
	io.ReadCloser
}

func (FakeErrorStream) Read(p []byte) (n int, err error) {
	return 0, errors.New("some Read() error")
}

func (FakeErrorStream) Close() error {
	return nil
}

func TestStreamBroadcastRetriesAfterReadError(t *testing.T) {
	urlOpenCount := make(chan int)
	var counter int = 0;

	fakeUrlOpen := func(url string) (*MinimalHttpResponse, error) {
		counter += 1
		urlOpenCount <- counter
		return &MinimalHttpResponse{&FakeErrorStream{}}, nil
	}

	session := NewBroadcastSession(fakeUrlOpen, 2)
	session.retrySleepTime = 1 * time.Nanosecond
	go streamBroadcast(session)

	for {
		select {
		case timesOpened := <-urlOpenCount:
			if timesOpened < 2 {
				log("stream not opened enough times:", timesOpened)
				continue
			}
			close(session.quit)
			return
		case <-time.After(3 * time.Second):
			panic("timeout: did not retry enough times after error")
		}
	}
}


func TestStreamBroadcastResetsAfterErrorRecovery(t *testing.T) {
	urlOpenCount := make(chan int)
	var counter int = 0;

	fakeUrlOpen := func(url string) (*MinimalHttpResponse, error) {
		counter += 1
		urlOpenCount <- counter
		var ret error
		// Only return an error on the first call.
		if counter == 1 {
			ret = errors.New("error to check reset")
		} else {
			ret = nil
		}
		return &MinimalHttpResponse{&FakeStream{}}, ret
	}

	session := NewBroadcastSession(fakeUrlOpen, 3)
	session.retrySleepTime = 1 * time.Nanosecond
	go streamBroadcast(session)

	for {
		select {
		case <-urlOpenCount:
			// TODO: fix this test so it actually verifies the reset.
			// There is some timing error here where it always passes :/
			if session.retryCount != 0 {
				log("retry count has not been reset yet")
				continue
			}
			close(session.quit)
			return
		case <-time.After(3 * time.Second):
			panic("timeout: did not retry enough times after error")
		}
	}
}
