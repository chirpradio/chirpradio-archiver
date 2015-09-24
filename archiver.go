package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const broadcastBuffSize = 1024 * 64  // 64Kb of data


func log(args ...interface{}) {
	t := time.Now()
	fmt.Printf("%d-%02d-%02dT%02d:%02d ",
		t.Year(), t.Month(), t.Day(),
		t.Hour(), t.Minute())
	fmt.Println(args...)
}


type MinimalHttpResponse struct {
	Body io.ReadCloser
}

type BroadcastSession interface {
	OpenUrl(url string) (*MinimalHttpResponse, error)
	StreamUrl() string
	Broadcast() chan []byte
	Quit() chan bool
	RetryCount() int
	MaxRetries() int
	IncrementRetry()
	ResetRetryCount()
}

type ChirpBroadcastSession struct {
	streamUrl string
	broadcast chan []byte
	maxRetries int
	quit chan bool
	retryCount int
	retrySleepTime time.Duration
}

func (sess *ChirpBroadcastSession) IncrementRetry() {
	time.Sleep(sess.retrySleepTime)
	sess.retryCount += 1
	log("Retrying...", sess.retryCount)
}

func (*ChirpBroadcastSession) OpenUrl(url string) (*MinimalHttpResponse, error) {
	response, err := http.Get(url)
	if err != nil {
		return &MinimalHttpResponse{}, err
	}
	return &MinimalHttpResponse{response.Body}, err
}

func (sess *ChirpBroadcastSession) StreamUrl() string {
	return sess.streamUrl
}

func (sess *ChirpBroadcastSession) Broadcast() chan []byte {
	return sess.broadcast
}

func (sess *ChirpBroadcastSession) Quit() chan bool {
	return sess.quit
}

func (sess *ChirpBroadcastSession) MaxRetries() int {
	return sess.maxRetries
}

func (sess *ChirpBroadcastSession) RetryCount() int {
	return sess.retryCount
}

func (sess *ChirpBroadcastSession) ResetRetryCount() {
	sess.retryCount = 0
}

func NewChirpBroadcastSession(
		streamUrl string, maxRetries int) BroadcastSession {
	broadcast := make(chan []byte, broadcastBuffSize)
	quit := make(chan bool)
	retryCount := 0
	retrySleepTime := 2 * time.Second

	return &ChirpBroadcastSession{
		streamUrl, broadcast, maxRetries, quit,
		retryCount, retrySleepTime}
}


func streamBroadcast(session BroadcastSession) error {

	if session.RetryCount() == session.MaxRetries() {
		log("streamBroadcast: too many error recovery retries")
		return errors.New("too many retries")
	}

	log("Streaming broadcast from", session.StreamUrl())
	response, err := session.OpenUrl(session.StreamUrl())

	if err != nil {
		log("Error while downloading", session.StreamUrl(), ":", err)
		session.IncrementRetry()
		return streamBroadcast(session)
	}
	defer response.Body.Close()

	for {
		buff := make([]byte, broadcastBuffSize)
		_, err := io.ReadFull(response.Body, buff)
		if err != nil {
			log("Error while streaming", session.StreamUrl(), ":", err)
			session.IncrementRetry()
			return streamBroadcast(session)
		}

		if session.RetryCount() > 0 {
			log("Recovered from last error")
			session.ResetRetryCount()
		}

		select {
		case <-session.Quit():
			log("stopping stream from quit signal")
			return nil
		case session.Broadcast() <-buff:
			continue
		}
	}

	return nil
}


type ArchiveWriter interface {
	OpenFile() (io.WriteCloser, error)
	FileName() string
	Broadcast() chan []byte
	Quit() chan int
}

type ArchiveFileWriter struct {
	broadcast chan []byte
	quit chan int
	fileName string
}

func (w *ArchiveFileWriter) OpenFile() (io.WriteCloser, error) {
	log("Opening new archive file:", w.fileName)
	file, err := os.Create(w.fileName)
	return file, err
}

func (w *ArchiveFileWriter) FileName() string {
	return w.fileName
}

func (w *ArchiveFileWriter) Broadcast() chan []byte {
	return w.broadcast
}

func (w *ArchiveFileWriter) Quit() chan int {
	return w.quit
}

func NewArchiveFileWriter(
		broadcast chan []byte, quit chan int,
		fileName string) (*ArchiveFileWriter) {
	return &ArchiveFileWriter{broadcast, quit, fileName}
}

func writeArchiveFile(writer ArchiveWriter) {
	output, err := writer.OpenFile()
	if err != nil {
		log("Error while creating", writer.FileName(), ":", err)
		panic(err)
	}

	for {
		select {
		case streamChunk := <-writer.Broadcast():
			output.Write(streamChunk)
		case <-writer.Quit():
			output.Close()
		}
	}
}


type ArchiveConfig interface {
	FileName(dest string, ts time.Time) string
	Dest(ts time.Time) string
	WriteFile(writer ArchiveWriter)
}

type ChirpArchiveConfig struct {
	rootDir string
}

func (archive *ChirpArchiveConfig) Dest(ts time.Time) string {
	prefix := fmt.Sprintf("%s/%d/%02d", archive.rootDir, ts.Year(), ts.Month())
	err := os.MkdirAll(prefix, 0744)
	if err != nil {
		panic(err)
	}
	return prefix
}

func (*ChirpArchiveConfig) FileName(dest string, ts time.Time) string {
	// TODO: protect against overwriting existing files.
	return fmt.Sprintf(
		"%s/chirpradio_%d-%02d-%02d_%02d%02d%02d.mp3",
		dest,
		ts.Year(), ts.Month(), ts.Day(),
		ts.Hour(), ts.Minute(), ts.Second(),
	)
}

func (archive *ChirpArchiveConfig) WriteFile(writer ArchiveWriter) {
	// TODO: maybe move the writeArchiveFile implementation over here :)
	writeArchiveFile(writer)
}

func NewChirpArchiveConfig(rootDir string) (*ChirpArchiveConfig) {
	return &ChirpArchiveConfig{rootDir}
}

func rotateArchiveFile(
		broadcast chan []byte, ts time.Time, archive ArchiveConfig) chan int {
	dest := archive.Dest(ts)
	fileName := archive.FileName(dest, ts)
	archiveChan := make(chan int)

	writer := NewArchiveFileWriter(broadcast, archiveChan, fileName)
	go archive.WriteFile(writer)

	return archiveChan
}


func main() {
	var url string = "http://chirpradio.org/stream"
	flag.StringVar(
		&url, "url", url, "URL to the CHIRP Radio broadcast stream.")

	var archiveDest = "./archives"
	flag.StringVar(
		&archiveDest, "dest", archiveDest,
		"Directory to write archives to. This must exist and be writable.")

	flag.Parse()

	maxErrorRetries := 8
	session := NewChirpBroadcastSession(url, maxErrorRetries)
	broadcast := session.Broadcast()
	go streamBroadcast(session)

	archive := NewChirpArchiveConfig(archiveDest)
	archiveChan := rotateArchiveFile(broadcast, time.Now(), archive)

	// TODO: force Chicago time so files are always in sync with the broadcast.
	ticker := time.NewTicker(1 * time.Second)

	// Save the broadcast to disk, rotating the archive file at the start
	// of every hour.
	for {
		// The Go docs say that this might drop ticks for slow receivers.
		// TODO: address dropped ticks somehow?
		tick := <-ticker.C
		if tick.Minute() == 0 && tick.Second() == 0 {
			close(archiveChan)
			archiveChan = rotateArchiveFile(broadcast, tick, archive)
		}
	}
}
