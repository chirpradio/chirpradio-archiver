package main

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"strings"
	"testing"
	"time"
)

var fakeStreamUrl = "http://not-chirpradio.org/"


func TestRotateArchiveFile(t *testing.T) {
	writerCalled := make(chan bool)

	prefix := "/archive-dir-prefix"
	broadcast := make(chan []byte, broadcastBuffSize)
	ts := time.Date(2015, time.September, 18, 23, 0, 0, 0, time.UTC)

	writer := func (info ArchiveWriter) error {
		fileIsOk := strings.HasSuffix(info.FileName(),
			"chirpradio_2015-09-18_230000.mp3")
		if !fileIsOk {
			t.Error("Unexpected suffix:", info.FileName())
		}

		dirIsOk := strings.HasPrefix(info.FileName(), prefix)
		if !dirIsOk {
			t.Error("Unexpected prefix:", info.FileName())
		}

		writerCalled <- true
		close(writerCalled)
		return nil
	}

	rotateArchiveFile(NewArchiveConfig(prefix, broadcast, ts, writer))

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

type MockArchiveWriter struct {
	broadcast chan []byte
	quit chan int
}

func (w *MockArchiveWriter) OpenFile() (io.WriteCloser, error) {
	return FakeOpener{}, nil
}

func (w *MockArchiveWriter) FileName() string {
	return "mock_archive_file.mp3"
}

func (w *MockArchiveWriter) Broadcast() chan []byte {
	return w.broadcast
}

func (w *MockArchiveWriter) Quit() chan int {
	return w.quit
}

func NewMockArchiveWriter() (*MockArchiveWriter) {
	return &MockArchiveWriter{
		broadcast: make(chan []byte),
		quit: make(chan int)}
}

func TestWriteArchiveFile(t *testing.T) {
	writer := NewMockArchiveWriter()
	go writeArchiveFile(writer)

	// Send some data through the broadcast channel.
	writer.Broadcast() <- []byte{0, 0}
	// TODO: figure out how to test that some output was written.
	close(writer.Quit())
}


type MockArchiveErrorWriter struct {
	MockArchiveWriter
	broadcast chan []byte
	quit chan int
}

func (w *MockArchiveErrorWriter) OpenFile() (io.WriteCloser, error) {
	return FakeOpener{}, errors.New("some error")
}

func (w *MockArchiveErrorWriter) Broadcast() chan []byte {
	return w.broadcast
}

func (w *MockArchiveErrorWriter) Quit() chan int {
	return w.quit
}

func NewMockArchiveErrorWriter() (*MockArchiveErrorWriter) {
	return &MockArchiveErrorWriter{
		broadcast: make(chan []byte),
		quit: make(chan int)}
}

func TestWriteArchiveFileWithError(t *testing.T) {
	writer := NewMockArchiveErrorWriter()
	// Close the channel in case the implementation doesn't return early as
	// expected.
	close(writer.Quit())

	result := writeArchiveFile(writer)

	if result == nil {
		t.Error("Unexpected result")
	}
}

func NewFakeStream() io.ReadCloser {
	// Make a fake stream chunk for the archive writer to consume.
	buf := bytes.NewBufferString(strings.Repeat("x", broadcastBuffSize))
	return ioutil.NopCloser(buf)
}

func NewFakeErrorStream() io.ReadCloser {
	// Make a stream chunk that is too short and thus will create EOF.
	buf := bytes.NewBufferString("x")
	return ioutil.NopCloser(buf)
}

type MockBroadcastSession struct {
	urlOpened chan bool
	broadcast chan []byte
	quit chan bool
}

func (*MockBroadcastSession) IncrementRetry() {
}

func (sess *MockBroadcastSession) OpenUrl(url string) (*MinimalHttpResponse, error) {
	sess.urlOpened <-true
	return &MinimalHttpResponse{NewFakeStream()}, nil
}

func (*MockBroadcastSession) StreamUrl() string {
	return fakeStreamUrl
}

func (sess *MockBroadcastSession) Broadcast() chan []byte {
	return sess.broadcast
}

func (sess *MockBroadcastSession) Quit() chan bool {
	return sess.quit
}

func (*MockBroadcastSession) MaxRetries() int {
	return 1
}

func (*MockBroadcastSession) RetryCount() int {
	return 0
}

func (*MockBroadcastSession) ResetRetryCount() {
}

func NewMockBroadcastSession() *MockBroadcastSession {
	return &MockBroadcastSession{
		urlOpened: make(chan bool),
		broadcast: make(chan []byte),
		quit: make(chan bool)}
}

func TestStreamBroadcast(t *testing.T) {
	session := NewMockBroadcastSession()
	go streamBroadcast(session)

	// TODO: figure out how to test that the stream gets sent to the broadcast
	// channel.

	for {
		select {
		case <-session.urlOpened:
			close(session.Quit())
			return
		case <-time.After(3 * time.Second):
			panic("timeout: streamBroadcast should open a URL")
		}
	}
}


type MockCounterBroadcastSession struct {
	*MockBroadcastSession
	counter int
	urlOpenCount chan int
	retryCount int
}

func (sess *MockCounterBroadcastSession) OpenUrl(url string) (*MinimalHttpResponse, error) {
	sess.counter += 1
	sess.urlOpenCount <- sess.counter
	return &MinimalHttpResponse{NewFakeStream()}, errors.New("some error")
}

func (sess *MockCounterBroadcastSession) IncrementRetry() {
	sess.retryCount += 1
}

func (sess *MockCounterBroadcastSession) MaxRetries() int {
	return 2
}

func (sess *MockCounterBroadcastSession) RetryCount() int {
	return sess.retryCount
}

func NewMockCounterBroadcastSession() *MockCounterBroadcastSession {
	return &MockCounterBroadcastSession{
		MockBroadcastSession: NewMockBroadcastSession(),
		retryCount: 0,
		urlOpenCount: make(chan int),
		counter: 0}
}

func TestStreamBroadcastRetriesAfterOpenError(t *testing.T) {
	session := NewMockCounterBroadcastSession()
	go streamBroadcast(session)

	for {
		select {
		case timesOpened := <-session.urlOpenCount:
			if timesOpened < 2 {
				log("stream not opened enough times:", timesOpened)
				continue
			}
			close(session.Quit())
			return
		case <-time.After(3 * time.Second):
			panic("timeout: did not retry enough times after error")
		}
	}
}


type MockReadErrorBroadcastSession struct {
	*MockBroadcastSession
	counter int
	urlOpenCount chan int
	retryCount int
}

func (sess *MockReadErrorBroadcastSession) OpenUrl(url string) (*MinimalHttpResponse, error) {
	sess.counter += 1
	sess.urlOpenCount <- sess.counter
	return &MinimalHttpResponse{NewFakeErrorStream()}, nil
}

func (sess *MockReadErrorBroadcastSession) IncrementRetry() {
	sess.retryCount += 1
}

func (sess *MockReadErrorBroadcastSession) MaxRetries() int {
	return 2
}

func (sess *MockReadErrorBroadcastSession) RetryCount() int {
	return sess.retryCount
}

func NewMockReadErrorBroadcastSession() *MockReadErrorBroadcastSession {
	return &MockReadErrorBroadcastSession{
		MockBroadcastSession: NewMockBroadcastSession(),
		retryCount: 0,
		urlOpenCount: make(chan int),
		counter: 0}
}

func TestStreamBroadcastRetriesAfterReadError(t *testing.T) {
	session := NewMockReadErrorBroadcastSession()
	go streamBroadcast(session)

	for {
		select {
		case timesOpened := <-session.urlOpenCount:
			if timesOpened < 2 {
				log("stream not opened enough times:", timesOpened)
				continue
			}
			close(session.Quit())
			return
		case <-time.After(3 * time.Second):
			panic("timeout: did not retry enough times after error")
		}
	}
}


type MockErrorRecoveryBroadcastSession struct {
	*MockBroadcastSession
	counter int
	wasReset chan bool
	retryCount int
}

func (sess *MockErrorRecoveryBroadcastSession) OpenUrl(url string) (*MinimalHttpResponse, error) {
	sess.counter += 1
	var ret error
	// Only return an error on the first call.
	if sess.counter == 1 {
		log("returning error on first call")
		ret = errors.New("error to check reset")
	} else {
		log("returning nil error on call:", sess.counter)
		ret = nil
	}
	return &MinimalHttpResponse{NewFakeStream()}, ret
}

func (sess *MockErrorRecoveryBroadcastSession) IncrementRetry() {
	sess.retryCount += 1
}

func (sess *MockErrorRecoveryBroadcastSession) MaxRetries() int {
	return 3
}

func (sess *MockErrorRecoveryBroadcastSession) RetryCount() int {
	return sess.retryCount
}

func (sess *MockErrorRecoveryBroadcastSession) ResetRetryCount() {
	log("resetting retry count")
	sess.wasReset <-true
	sess.retryCount = 0
}

func NewMockErrorRecoveryBroadcastSession() *MockErrorRecoveryBroadcastSession {
	return &MockErrorRecoveryBroadcastSession{
		MockBroadcastSession: NewMockBroadcastSession(),
		retryCount: 0,
		wasReset: make(chan bool),
		counter: 0}
}

func TestStreamBroadcastResetsAfterErrorRecovery(t *testing.T) {
	session := NewMockErrorRecoveryBroadcastSession()
	go streamBroadcast(session)

	for {
		select {
		case <-session.wasReset:
			if session.counter != 2 {
				t.Error("Unexpected counter:", session.counter)
			}
			close(session.Quit())
			return
		case <-time.After(3 * time.Second):
			panic("timeout: did not reset after error")
		}
	}
}
