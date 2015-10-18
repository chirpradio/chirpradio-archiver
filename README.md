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
set. Put something like this in your shell profile:

    export GOPATH=$HOME/golang

Also make sure all Go executables are on your path:

    export PATH=$PATH:$GOPATH/bin

Install the archiver:

    go get github.com/chirpradio/chirpradio-archiver

Now you can check out the options for the archiver:

    chirpradio-archiver -help

Here's an example of starting the archiver process:

    chirpradio-archiver -url=http://chirpradio.org/stream -dest=/path/to/archives

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

## Development

To contribute new features to this library, you can set yourself up the same
way as above. That is, set your `$GOPATH`, add `$GOPATH/bin` to your `$PATH`,
and run `go get ...` to fetch the code.
Change into the source code directory:

    cd $GOPATH/src/github.com/chirpradio/chirpradio-archiver

Here's how to run the tests:

    go test

If you need to add any new dependencies, just run:

    go get ./...

## Bugs?

Probably! You can report them on the github issue tracker.
