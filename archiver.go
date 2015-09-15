package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)


const broadcastBuffSize = 1024 * 8  // 8Kb of data
var broadcast = make(chan []byte, broadcastBuffSize)


func log(args ...interface{}) {
	t := time.Now()
	fmt.Printf("%d-%02d-%02dT%02d:%02d ",
		t.Year(), t.Month(), t.Day(),
		t.Hour(), t.Minute())
	fmt.Println(args...)
}


func streamBroadcast() {
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


func writeArchiveFile(quit chan int, fileName string) {
	log("Opening new archive file:", fileName)
	output, err := os.Create(fileName)
	if err != nil {
		log("Error while creating file", err)
		return
	}

	for {
		select {
		case streamChunk := <-broadcast:
			output.Write(streamChunk)
		case <- quit:
			output.Close()
			return
		}
	}
}


func rotateArchiveFile(ts time.Time) chan int {
	archiveFileName := fmt.Sprintf(
		// TODO: protect against overwriting existing files.
		"archives/chirpradio_%d-%02d-%02d_%02d%02d%02d.mp3",
		ts.Year(), ts.Month(), ts.Day(), ts.Hour(),
		ts.Minute(), ts.Second(),
	)

	archiveChan := make(chan int)
	go writeArchiveFile(archiveChan, archiveFileName)
	return archiveChan
}


func main() {
	go streamBroadcast()
	archiveChan := rotateArchiveFile(time.Now())
	// TODO: force Chicago time to always be in sync with the broadcast.
	ticker := time.NewTicker(1 * time.Second)

	for {
		// The Go docs say that this might drop ticks for slow receivers.
		// TODO: address that somehow?
		tick := <-ticker.C
		if tick.Minute() == 0 && tick.Second() == 0 {
			// Rotate the file at the start of every hour.
			close(archiveChan)
			archiveChan = rotateArchiveFile(tick)
		}
	}
}
