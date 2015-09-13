package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)


var broadcast = make(chan []byte, 1024)  // buffered


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
		buff := make([]byte, 1024)
		_, err := io.ReadFull(response.Body, buff)
		if err != nil {
			log("Error while streaming", url, "-", err)
			return
		}
		broadcast <- buff
	}
}


func writeArchiveFile(quit chan int, fileName string) {
	log("Writing to:", fileName)
	output, err := os.Create(fileName)
	if err != nil {
		log("Error while creating file", err)
		return
	}
	defer output.Close()

	for {
		select {
		case streamChunk := <- broadcast:
			output.Write(streamChunk)
		case <- quit:
			log("exiting archive writer")
			return
		}
	}
}


func main() {
	go streamBroadcast()

	for {
		localTime := time.Now()
		archiveFileName := fmt.Sprintf(
			"archives/chirpradio_%d-%02d-%02d_%02d%02d%02d.mp3",
			localTime.Year(), localTime.Month(),
			localTime.Day(), localTime.Hour(),
			localTime.Minute(), localTime.Second(),
		)

		writeArchive := make(chan int)
		go writeArchiveFile(writeArchive, archiveFileName)
		time.Sleep(time.Second * 30)
		close(writeArchive)
	}
}
