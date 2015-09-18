package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const broadcastBuffSize = 1024 * 8  // 8Kb of data


func log(args ...interface{}) {
	t := time.Now()
	fmt.Printf("%d-%02d-%02dT%02d:%02d ",
		t.Year(), t.Month(), t.Day(),
		t.Hour(), t.Minute())
	fmt.Println(args...)
}


func streamBroadcast(broadcast chan []byte) {
	url := "http://chirpradio.org/stream"
	log("Streaming broadcast from", url)
	response, err := http.Get(url)
	if err != nil {
		log("Error while downloading", url, "-", err)
		return
	}
	defer response.Body.Close()

	for {
		buff := make([]byte, broadcastBuffSize)
		_, err := io.ReadFull(response.Body, buff)
		if err != nil {
			log("Error while streaming", url, "-", err)
			return
		}
		broadcast <- buff
	}
}


type archiveInfo struct {
	broadcast chan []byte
	quit chan int
	fileName string
}

type fileOpener func(string) (io.WriteCloser, error)
type archiveWriter func(info archiveInfo, openFile fileOpener) error


func createFile(name string) (io.WriteCloser, error) {
	file, err := os.Create(name)
	var writer io.WriteCloser = file
	return writer, err
}


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
	go writeFile(
		archiveInfo{broadcast, archiveChan, archiveFileName},
		createFile)
	return archiveChan
}


func main() {
	var broadcast = make(chan []byte, broadcastBuffSize)

	go streamBroadcast(broadcast)
	archiveChan := rotateArchiveFile(
		broadcast, time.Now(), writeArchiveFile)

	// TODO: force Chicago time to always be in sync with the broadcast.
	ticker := time.NewTicker(1 * time.Second)

	// Save the broadcast to disk, rotating the archive file at the start
	// of every hour.
	for {
		// The Go docs say that this might drop ticks for slow receivers.
		// TODO: address that somehow?
		tick := <-ticker.C
		if tick.Minute() == 0 && tick.Second() == 0 {
			close(archiveChan)
			archiveChan = rotateArchiveFile(
				broadcast, tick, writeArchiveFile)
		}
	}
}
