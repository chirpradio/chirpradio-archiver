package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
	log "github.com/sirupsen/logrus"
)

const broadcastBuffSize = 1024 * 64  // 64Kb of data


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
	log.WithFields(log.Fields{
		"retryCount": sess.retryCount,
	}).Info("Retrying...")
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
		log.Info("streamBroadcast: too many error recovery retries")
		return errors.New("too many retries")
	}

	log.WithFields(log.Fields{
		"url": session.StreamUrl(),
	}).Info("Streaming broadcast")
	response, err := session.OpenUrl(session.StreamUrl())

	if err != nil {
		log.WithFields(log.Fields{
			"url": session.StreamUrl(),
			"err": err,
		}).Info("Error while downloading")
		session.IncrementRetry()
		return streamBroadcast(session)
	}
	defer response.Body.Close()

	for {
		buff := make([]byte, broadcastBuffSize)
		_, err := io.ReadFull(response.Body, buff)
		if err != nil {
			log.WithFields(log.Fields{
				"url": session.StreamUrl(),
				"err": err,
			}).Info("Error while streaming")
			session.IncrementRetry()
			return streamBroadcast(session)
		}

		if session.RetryCount() > 0 {
			log.Info("Recovered from last error")
			session.ResetRetryCount()
		}

		select {
		case <-session.Quit():
			log.Info("stopping stream from quit signal")
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
	log.WithFields(log.Fields{
		"file": w.fileName,
	}).Debug("Opening new archive file")
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


type ArchiveConfig interface {
	FileName(dest string, ts time.Time) string
	Dest(ts time.Time) string
	WriteFile(writer ArchiveWriter)
}

type ChirpArchiveConfig struct {
	rootDir string
}

func (archive *ChirpArchiveConfig) Dest(ts time.Time) string {
	// Organize archive files in YYYY/MM/DD directories.
	prefix := fmt.Sprintf(
		"%s/%d/%02d/%02d", archive.rootDir, ts.Year(), ts.Month(), ts.Day())
	err := os.MkdirAll(prefix, 0755)
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
	output, err := writer.OpenFile()
	if err != nil {
		log.WithFields(log.Fields{
			"file": writer.FileName(),
			"err": err,
		}).Info("Error while creating")
		panic(err)
	}

	for {
		select {
		case streamChunk := <-writer.Broadcast():
			output.Write(streamChunk)
		case <-writer.Quit():
			output.Close()
			return
		}
	}
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
		&url, "url", url,
		"CHIRP Radio broadcast stream URL. On the internal network this should be a " +
		"URL to the streaming appliance.")

	var archiveDest = "./archives"
	flag.StringVar(
		&archiveDest, "dest", archiveDest,
		"Directory to write archives to. This must exist and be writable.")

	quiet := flag.Bool("quiet", false, "When true, debug logging will be hidden.")

	flag.Parse()

	log.SetFormatter(
		&log.TextFormatter{
			// This is a weird Go-specific reference date.
			// https://stackoverflow.com/questions/36206187/logrus-timestamp-formatting
			TimestampFormat: "2006-01-02 15:04:05",
			FullTimestamp: true})

	if *quiet {
		log.SetLevel(log.InfoLevel)
	} else {
		log.SetLevel(log.DebugLevel)
	}

	log.Info("Starting archiver")

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
