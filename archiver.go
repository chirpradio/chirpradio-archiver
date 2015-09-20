package main

import (
	"errors"
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


type BroadcastConfig struct {
	broadcast chan []byte
	openUrl urlOpener
	maxRetries int
	quit chan bool
	retryCount int
	retrySleepTime time.Duration
}

func (c *BroadcastConfig) IncrementRetry() {
	time.Sleep(c.retrySleepTime)
	c.retryCount += 1
	log("Retrying...", c.retryCount)
}

func NewBroadcastConfig(openUrl urlOpener, maxRetries int) *BroadcastConfig {
	broadcast := make(chan []byte, broadcastBuffSize)
	quit := make(chan bool)
	retryCount := 0
	retrySleepTime := 2 * time.Second

	return &BroadcastConfig{
		broadcast, openUrl, maxRetries, quit, retryCount, retrySleepTime}
}


func streamBroadcast(config *BroadcastConfig) error {

	if config.retryCount == config.maxRetries {
		log("streamBroadcast: too many error recovery retries")
		return errors.New("too many retries")
	}

	url := "http://chirpradio.org/stream"
	log("Streaming broadcast from", url)
	response, err := config.openUrl(url)

	if err != nil {
		log("Error while downloading", url, ":", err)
		config.IncrementRetry()
		return streamBroadcast(config)
	}
	defer response.Body.Close()

	for {
		buff := make([]byte, broadcastBuffSize)
		_, err := io.ReadFull(response.Body, buff)
		if err != nil {
			log("Error while streaming", url, ":", err)
			config.IncrementRetry()
			return streamBroadcast(config)
		}

		// We've successfully recovered from the last persistent error
		// so reset the retry count.
		config.retryCount = 0

		select {
		case <-config.quit:
			log("stopping stream from quit signal")
			return nil
		case config.broadcast <-buff:
			continue
		}
	}
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
		// TODO: think of a way to give precedence to the broadcast channel
		// in case the quit channel is ready simultaneously.
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
	openUrl := func(url string) (*MinimalHttpResponse, error) {
		response, err := http.Get(url)
		if err != nil {
			return &MinimalHttpResponse{}, err
		}
		return &MinimalHttpResponse{response.Body}, err
	}
	maxErrorRetries := 10
	config := NewBroadcastConfig(openUrl, maxErrorRetries)
	go streamBroadcast(config)

	archiveChan := rotateArchiveFile(
		config.broadcast, time.Now(), writeArchiveFile)

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
				config.broadcast, tick, writeArchiveFile)
		}
	}
}
