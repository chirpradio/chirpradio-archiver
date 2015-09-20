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

type urlOpener func(string) (*MinimalHttpResponse, error)


type BroadcastSession struct {
	streamUrl string
	broadcast chan []byte
	openUrl urlOpener
	maxRetries int
	quit chan bool
	retryCount int
	retrySleepTime time.Duration
}

func (c *BroadcastSession) IncrementRetry() {
	time.Sleep(c.retrySleepTime)
	c.retryCount += 1
	log("Retrying...", c.retryCount)
}

func NewBroadcastSession(
		streamUrl string, openUrl urlOpener, maxRetries int) *BroadcastSession {
	broadcast := make(chan []byte, broadcastBuffSize)
	quit := make(chan bool)
	retryCount := 0
	retrySleepTime := 2 * time.Second

	return &BroadcastSession{
		streamUrl, broadcast, openUrl, maxRetries, quit,
		retryCount, retrySleepTime}
}


func streamBroadcast(session *BroadcastSession) error {

	if session.retryCount == session.maxRetries {
		log("streamBroadcast: too many error recovery retries")
		return errors.New("too many retries")
	}

	log("Streaming broadcast from", session.streamUrl)
	response, err := session.openUrl(session.streamUrl)

	if err != nil {
		log("Error while downloading", session.streamUrl, ":", err)
		session.IncrementRetry()
		return streamBroadcast(session)
	}
	defer response.Body.Close()

	for {
		buff := make([]byte, broadcastBuffSize)
		_, err := io.ReadFull(response.Body, buff)
		if err != nil {
			log("Error while streaming", session.streamUrl, ":", err)
			session.IncrementRetry()
			return streamBroadcast(session)
		}

		// We've successfully recovered from the last persistent error
		// so reset the retry count.
		session.retryCount = 0

		select {
		case <-session.quit:
			log("stopping stream from quit signal")
			return nil
		case session.broadcast <-buff:
			continue
		}
	}

	return nil
}


type archiveInfo struct {
	broadcast chan []byte
	quit chan int
	fileName string
}

type fileOpener func(string) (io.WriteCloser, error)

func writeArchiveFile(info archiveInfo, openFile fileOpener) error {
	log("Opening new archive file:", info.fileName)
	output, err := openFile(info.fileName)
	if err != nil {
		log("Error while creating", info.fileName, ":", err)
		return err
	}

	for {
		select {
		case streamChunk := <-info.broadcast:
			output.Write(streamChunk)
		case <-info.quit:
			output.Close()
			return nil
		}
	}
}


type archiveWriter func(info archiveInfo, openFile fileOpener) error

func rotateArchiveFile(
		broadcast chan []byte, ts time.Time,
		writeFile archiveWriter) chan int {

	archiveFileName := fmt.Sprintf(
		// TODO: protect against overwriting existing files.
		"archives/chirpradio_%d-%02d-%02d_%02d%02d%02d.mp3",
		ts.Year(), ts.Month(), ts.Day(), ts.Hour(),
		ts.Minute(), ts.Second(),
	)
	archiveChan := make(chan int)

	createFile := func(name string) (io.WriteCloser, error) {
		file, err := os.Create(name)
		var writer io.WriteCloser = file
		return writer, err
	}

	go writeFile(
		archiveInfo{broadcast, archiveChan, archiveFileName},
		createFile)

	return archiveChan
}


func main() {
	var url string = "http://chirpradio.org/stream"
	flag.StringVar(
		&url, "url", url, "URL to the CHIRP Radio broadcast stream")
	flag.Parse()

	openUrl := func(url string) (*MinimalHttpResponse, error) {
		response, err := http.Get(url)
		if err != nil {
			return &MinimalHttpResponse{}, err
		}
		return &MinimalHttpResponse{response.Body}, err
	}
	maxErrorRetries := 10
	session := NewBroadcastSession(url, openUrl, maxErrorRetries)
	go streamBroadcast(session)

	archiveChan := rotateArchiveFile(
		session.broadcast, time.Now(), writeArchiveFile)

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
			archiveChan = rotateArchiveFile(
				session.broadcast, tick, writeArchiveFile)
		}
	}
}
