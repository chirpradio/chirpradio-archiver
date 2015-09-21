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

		// We've successfully recovered from the last persistent error
		// so reset the retry count.
		session.ResetRetryCount()

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
		fileName string) (ArchiveWriter) {
	return &ArchiveFileWriter{broadcast, quit, fileName}
}

type fileOpener func(string) (io.WriteCloser, error)

func writeArchiveFile(writer ArchiveWriter) error {
	output, err := writer.OpenFile()
	if err != nil {
		log("Error while creating", writer.FileName(), ":", err)
		return err
	}

	for {
		select {
		case streamChunk := <-writer.Broadcast():
			output.Write(streamChunk)
		case <-writer.Quit():
			output.Close()
			return nil
		}
	}
}


type writeArchiveHandler func(writer ArchiveWriter) error

type ArchiveConfig struct {
	dest string
	broadcast chan []byte
	ts time.Time
	writeFile writeArchiveHandler
}

func NewArchiveConfig(
		dest string, broadcast chan []byte, ts time.Time,
		writeFile writeArchiveHandler) (*ArchiveConfig) {
	return &ArchiveConfig{dest, broadcast, ts, writeFile}
}

func rotateArchiveFile(archive *ArchiveConfig) chan int {

	// TODO: split directory into YYYY/MM
	// TODO: protect against overwriting existing files.
	fileName := fmt.Sprintf(
		"%s/chirpradio_%d-%02d-%02d_%02d%02d%02d.mp3",
		archive.dest,
		archive.ts.Year(), archive.ts.Month(), archive.ts.Day(),
		archive.ts.Hour(), archive.ts.Minute(), archive.ts.Second(),
	)
	archiveChan := make(chan int)

	writer := NewArchiveFileWriter(archive.broadcast, archiveChan, fileName)
	go archive.writeFile(writer)

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
	go streamBroadcast(session)

	archive := NewArchiveConfig(
		archiveDest, session.Broadcast(), time.Now(), writeArchiveFile)
	archiveChan := rotateArchiveFile(archive)

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
			archive.ts = tick
			archiveChan = rotateArchiveFile(archive)
		}
	}
}
