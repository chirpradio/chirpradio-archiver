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

Install [golang](http://golang.org/) (>= 1.14) and make sure your `$GOPATH` is
set. Put something like this in your shell profile:

    export GOPATH=$HOME/golang

Also make sure all Go executables are on your path:

    export PATH=$PATH:$GOPATH/bin

Install (or upgrade) the archiver on a production system:

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

If the archiver encounters an error, try restarting it.
As an admin user, type this:

    sudo service chirpradio-archiver restart

Here's how to upgrade the service to a newer version of the archiver.
As the `archiver` user, update the code:

    sudo su archiver
    go get -u github.com/chirpradio/chirpradio-archiver

then restart the service as explained above.

The upstart script (in `/etc/init/chirpradio-archiver.conf`) configures how the
`chirpradio-archiver` command is executed. Here is an example of the `exec` line
in that script:

    chirpradio-archiver \
        -url=http://192.X.X.X:8000/ \
        -dest=/mnt/disk_array/archives/ \
        -quiet >> $LOGFILE 2>&1

The `-url` in this case is a special streaming server that the broadcasting
applicance is configured to run within CHIRP's internal network.

**IMPORTANT**: This internal streaming server is only designed to handle one listener
which means you could knock down an archiver if you tried to connect to it
twice (e.g. for testing).

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

To contribute new features to this library, change into the source code directory:

    git clone <this repo>
    cd chirpradio-archiver

Here's how to run the tests:

    go test

If you want to add new dependencies, add them to the source code and re-run `go test`. Be sure to commit the changes to `go.mod` and `go.sum`.

The `chirpradio-archiver` binary is only compiled when you run `go build`
so, for development, run it directly to test local changes:

    go run archiver.go -help

## Bugs?

Probably! You can report them on the github issue tracker.

## Questions?

You can reach us at the [chirpdev Google Group](https://groups.google.com/forum/#!forum/chirpdev).
