# chirpradio-archiver

This is a long running program that listens to the live
[CHIRP Radio](http://chirpradio.org/) broadcast
stream and saves archive files to disk, one file per hour. It is the successor
to the archiving scripts in
[chirpradio-machine](https://github.com/chirpradio/chirpradio-machine/).

**IMPORTANT**: This is intended for internal station use only where we connect
directly to our streaming appliance. Please don't archive the live broadcast
yourself (unless testing) because CHIRP Radio pays per listener.

## Usage

Install [golang](http://golang.org/) (>= 1.4) and make sure your `$GOPATH` is
set. Something like this maybe:

    export GOPATH=$HOME/golang

Install the archiver's dependencies:

    go get github.com/DramaFever/go-logging

Clone the `chirpradio-archver` repository, and start archiving like this:

    cd chirpradio-archiver/
    go run archiver.go -url=http://chirpradio.org/stream -dest=/path/to/archives

This will write archive files like the following example:

    /path/to/archives/2015/09/chirpradio_2015-09-21_000000.mp3

Any non-existing timestamp directories will be created.

## Architecture

This program utilizes goroutines for robust and precise archiving. Here are some
notes:

1. A dedicated stream channel reads audio continously into a buffer so that no
   audio is ever lost.
2. Go's `time.Ticker` keeps the archive file rotation on time.
3. If there are errors when processing the stream, they are retried several
   times to recover if possible.

## Developers

To make changes or add features, you can run the test suite like this:

    go test

## Bugs?

Probably! You can report them on the github issue tracker.
