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

Install (or upgrade) the archiver:

    go get -u github.com/chirpradio/chirpradio-archiver

Now you can check out the options for the archiver:

    chirpradio-archiver -help

Here's an example of starting the archiver process:

    chirpradio-archiver -url=http://chirpradio.org/stream -dest=/path/to/archives

This will write archive files like the following example:

    /path/to/archives/2015/09/chirpradio_2015-09-21_000000.mp3

Any non-existing timestamp directories will be created.

## Deployment

The archiver is deployed to a dedicated Linux server in CHIRP's studio so that
it can connect directly to the streaming appliance, as opposed to making an
external Internet connection to the relayed stream. Here's what you need to know
about deploying and updating the service.

The `chirpradio-archiver` script is installed to the `$GOPATH` within
`/home/archiver` but is controlled by
[upstart](http://upstart.ubuntu.com/). The service starts when the machine boots
but here's how you would start it manually:

    sudo service chirpradio-archiver start

You can check `/home/archiver/log/archiver.log` to see its output or check
`/var/log/syslog` if there appears to be a more fatal error.

Here's how to upgrade the service to a newer version of the archiver.
As the `archiver` user, update the code:

    sudo su archiver
    go get -u github.com/chirpradio/chirpradio-archiver

As an admin user, restart the service:

    sudo service chirpradio-archiver stop
    sudo service chirpradio-archiver start

The upstart script (in `/etc/init/chirpradio-archiver.conf`) configures how the
`chirpradio-archiver` command is executed. Here is an example of the `exec` line
in that script:

    chirpradio-archiver \
        -url=http://192.X.X.X:8000/ \
        -dest=/mnt/disk_array/archives/ \
        -quiet >> $LOGFILE 2>&1

The `-url` in this case is a special streaming server that the broadcasting
applicance is configured to run within CHIRP's internal network.
**IMPORTANT**: this streaming server is only designed to handle one listener
which means you could knock down an archiver if you tried to connect to it
twice (for testing, or whatever).

The `-dest` in this case points to the RAID array where archive files are
stored.

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

The `chirpradio-archiver` binary is compiled when you run `go get ...` so
you need to run it directly if you want to test your local changes:

    go run archiver.go -help

## Bugs?

Probably! You can report them on the github issue tracker.

## Questions?

You can reach us at the [chirpdev Google Group](https://groups.google.com/forum/#!forum/chirpdev).
